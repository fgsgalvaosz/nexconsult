const puppeteer = require('puppeteer');
const extractorService = require('./extractorService');
const navigationService = require('./navigationService');
const CacheService = require('./cacheService');
const preloadService = require('./preloadService');
const config = require('../config');
const fs = require('fs').promises;
const { cnpjLogger, performanceLogger, createCorrelatedLogger } = require('../utils/logger');
const SolveCaptchaService = require('./solveCaptchaService');

// Optimized logging system
const isDevelopment = process.env.NODE_ENV !== 'production';
const logLevel = process.env.LOG_LEVEL || (isDevelopment ? 'debug' : 'info');

// Conditional logging functions for performance
const infoLog = (logLevel === 'debug' || logLevel === 'info') ? console.log : () => {};

// Intelligent cache service for CNPJ results
const cnpjCache = new CacheService({
    defaultTTL: 30 * 60 * 1000, // 30 minutes
    maxSize: 1000, // Maximum 1000 cached CNPJs
    cleanupInterval: 5 * 60 * 1000 // Cleanup every 5 minutes
});
// Removed axios and FormData - now using SolveCaptchaService

// Initialize resilient captcha service
const resilientCaptchaService = new SolveCaptchaService(config.SOLVE_CAPTCHA_API_KEY);

// Using SolveCaptchaService for resilient captcha handling

// Removed unused resilience classes - functionality moved to SolveCaptchaService

// All resilience functionality moved to SolveCaptchaService

// Browser pool for performance optimization
class BrowserPool {
    constructor(maxSize = 2) {
        this.pool = [];
        this.maxSize = maxSize;
        this.activeCount = 0;
    }

    async getBrowser() {
        if (this.pool.length > 0) {
            console.log('CNPJService: ⚡ Reusing browser from pool');
            return this.pool.pop();
        }

        if (this.activeCount < this.maxSize) {
            this.activeCount++;
            cnpjLogger.debug('Creating new browser instance');
            return await this.createBrowser();
        }

        // Wait for a browser to become available
        return new Promise((resolve) => {
            const checkPool = () => {
                if (this.pool.length > 0) {
                    resolve(this.pool.pop());
                } else {
                    setTimeout(checkPool, 100);
                }
            };
            checkPool();
        });
    }

    async returnBrowser(browser) {
        try {
            // Check if browser is still connected (melhorada para Codespace)
            let isConnected = false;
            try {
                // Método mais robusto para verificar conexão
                await browser.version();
                isConnected = true;
            } catch (e) {
                // Browser desconectado
                isConnected = false;
                cnpjLogger.debug({ err: e }, 'Browser connection check failed');
            }

            if (this.pool.length < this.maxSize && isConnected) {
                // Clean up the browser for reuse
                const pages = await browser.pages();
                for (const page of pages) {
                    if (!page.isClosed()) {
                        await page.close();
                    }
                }
                this.pool.push(browser);
                console.log('CNPJService: ⚡ Browser returned to pool');
            } else {
                if (isConnected) {
                    await browser.close();
                }
                this.activeCount--;
                console.log('CNPJService: Browser closed (pool full or browser disconnected)');
            }
        } catch (error) {
            cnpjLogger.error({ err: error }, 'Error managing browser in pool');
            this.activeCount--;
            // Tentar fechar browser mesmo com erro
            try {
                if (browser && !browser.isClosed()) {
                    await browser.close();
                }
            } catch (closeError) {
                cnpjLogger.debug({ err: closeError }, 'Error closing browser after pool error');
            }
        }
    }

    async createBrowser() {
        return await puppeteer.launch({
            headless: 'new',
            args: [
                '--no-sandbox',
                '--disable-setuid-sandbox',
                '--disable-dev-shm-usage',
                '--disable-blink-features=AutomationControlled',
                '--disable-features=VizDisplayCompositor'
            ],
            ignoreDefaultArgs: ['--enable-automation'],
            ignoreHTTPSErrors: true,
            timeout: 60000,
            defaultViewport: { width: 1920, height: 1080 }
        });
    }

    async cleanup() {
        const browsers = [...this.pool];
        this.pool = [];
        for (const browser of browsers) {
            await browser.close();
        }
        this.activeCount = 0;
    }
}

const browserPool = new BrowserPool();

class CNPJService {
    /**
     * Validate CNPJ format and check digit (Optimized version)
     */
    validateCNPJ(cnpj) {
        if (!cnpj) {
            return { valid: false, error: 'CNPJ é obrigatório' };
        }

        // Remove formatting
        const cleanCNPJ = cnpj.replace(/\D/g, '');

        // Check length
        if (cleanCNPJ.length !== 14) {
            return { valid: false, error: 'CNPJ deve ter 14 dígitos' };
        }

        // Check if all digits are the same
        if (/^(\d)\1+$/.test(cleanCNPJ)) {
            return { valid: false, error: 'CNPJ não pode ter todos os dígitos iguais' };
        }

        // Validate check digits (optimized)
        const digits = cleanCNPJ.split('').map(Number);

        // First check digit
        let sum = 0;
        let weight = 5;
        for (let i = 0; i < 12; i++) {
            sum += digits[i] * weight;
            weight = weight === 2 ? 9 : weight - 1;
        }
        const firstCheck = sum % 11 < 2 ? 0 : 11 - (sum % 11);

        if (digits[12] !== firstCheck) {
            return { valid: false, error: 'CNPJ inválido - primeiro dígito verificador incorreto' };
        }

        // Second check digit
        sum = 0;
        weight = 6;
        for (let i = 0; i < 13; i++) {
            sum += digits[i] * weight;
            weight = weight === 2 ? 9 : weight - 1;
        }
        const secondCheck = sum % 11 < 2 ? 0 : 11 - (sum % 11);

        if (digits[13] !== secondCheck) {
            return { valid: false, error: 'CNPJ inválido - segundo dígito verificador incorreto' };
        }

        return {
            valid: true,
            formatted: cleanCNPJ.replace(/^(\d{2})(\d{3})(\d{3})(\d{4})(\d{2})$/, '$1.$2.$3/$4-$5')
        };
    }

    /**
     * Perform complete CNPJ consultation with data extraction and retry logic
     * @param {string} cnpj - CNPJ number to consult
     * @param {string} apiKey - Optional API key for captcha solving
     * @returns {Promise<Object>} Complete consultation result with extracted data
     */
    async consultarCNPJ(cnpj, apiKey = null) {
        const maxRetries = 3;
        const retryDelay = 5000; // 5 seconds between retries

        for (let attempt = 1; attempt <= maxRetries; attempt++) {
            try {
                const result = await this.consultarCNPJSingleAttempt(cnpj, apiKey, attempt);
                return result;
            } catch (error) {
                const cacheKey = cnpj.replace(/[^\d]/g, '');

                // Clear cache on failure to ensure fresh attempt
                if (cnpjCache.delete(cacheKey)) {
                    console.log(`CNPJService: Cache cleared for CNPJ ${cnpj} after failure (attempt ${attempt})`);
                }

                if (attempt === maxRetries) {
                    console.error(`CNPJService: All ${maxRetries} attempts failed for CNPJ ${cnpj}`);
                    throw error;
                }

                console.log(`CNPJService: Attempt ${attempt}/${maxRetries} failed for CNPJ ${cnpj}. Retrying in ${retryDelay/1000}s...`);
                console.log(`CNPJService: Error: ${error.message}`);

                // Wait before retry
                await this.delay(retryDelay);
            }
        }
    }

    /**
     * Single attempt at CNPJ consultation
     * @param {string} cnpj - CNPJ number to consult
     * @param {string} apiKey - Optional API key for captcha solving
     * @param {number} attempt - Current attempt number
     * @returns {Promise<Object>} Complete consultation result with extracted data
     */
    async consultarCNPJSingleAttempt(cnpj, apiKey = null, attempt = 1) {
        const startTime = Date.now();
        let browser;
        try {
            // Validate CNPJ format first
            const validation = this.validateCNPJ(cnpj);
            if (!validation.valid) {
                throw new Error(`CNPJ inválido: ${validation.error}`);
            }

            // Use provided API key or default from config
            const chaveApi = apiKey || config.SOLVE_CAPTCHA_API_KEY;

            const executionId = `exec_${startTime}_${Math.random().toString(36).substring(7)}_attempt${attempt}`;
            const correlatedLogger = createCorrelatedLogger(cnpjLogger, executionId);

            correlatedLogger.info({ cnpj, executionId, attempt }, 'Starting complete consultation');

            // Check cache first for performance optimization (only on first attempt)
            const cacheKey = cnpj.replace(/[^\d]/g, ''); // Clean CNPJ for cache key
            if (attempt === 1) {
                const cachedResult = cnpjCache.get(cacheKey);
                if (cachedResult) {
                    correlatedLogger.info({ cnpj, executionId }, 'Cache hit - returning cached result');
                    return cachedResult;
                }
            } else {
                correlatedLogger.info({ cnpj, executionId, attempt }, 'Retry attempt - skipping cache check');
            }

            // Step 1: Perform the consultation with improved logic
            browser = await browserPool.getBrowser();
            const page = await browser.newPage();

            // PRELOAD OPTIMIZATION: Try to use pre-loaded page if available
            const preloadedPage = await preloadService.getFromCache(`consultation_page_${cnpj}`);
            if (preloadedPage && preloadedPage.data.ready) {
                console.log('CNPJService: Using pre-loaded consultation page');
            }

            // Setup performance-optimized request interception
            await navigationService.setupRequestInterception(page);

            // Interceptar respostas para detectar erros
            page.on('response', (response) => {
                const url = response.url();
                const status = response.status();

                // Log responses importantes
                if (url.includes('cnpj') || url.includes('consulta') || status >= 400) {
                    console.log(`CNPJService: 📥 Response: ${status} <- ${url}`);
                }

                // Detectar redirecionamentos ou erros
                if (status >= 300 && status < 400) {
                    console.log(`CNPJService: 🔄 Redirect ${status}: ${url}`);
                } else if (status >= 400) {
                    console.log(`CNPJService: ❌ HTTP Error ${status}: ${url}`);
                }
            });
            
            // Configure page using NavigationService
            await navigationService.configurePage(page);

            // Navigate directly to consultation page with CNPJ pre-filled
            await navigationService.navigateToConsultationPageWithCNPJ(page, cnpj);

            // Check if captcha is present before trying to solve it
            const hasCaptcha = await navigationService.checkIfCaptchaIsPresent(page);
            infoLog(`CNPJService: Captcha present: ${hasCaptcha}`);

            if (hasCaptcha) {
                // Solve hCaptcha only if present
                await this.solveCaptcha(page, chaveApi);
            } else {
                infoLog('CNPJService: No captcha detected, proceeding directly to form submission');
            }

            // Submit form and wait for results
            await this.submitFormAndWaitForResults(page);

            // Step 2: Extract data from the result page
            let dadosExtraidos = null;

            // Always try to extract data, even if resultPage is false
            try {
                // ADAPTIVE DELAY: Wait for page to be ready for extraction
                await this.smartDelay(page, 'dom_ready', 2000);

                // PARALLELIZATION: Execute multiple operations simultaneously
                const [content, currentUrl] = await Promise.all([
                    page.content(),
                    page.url()
                ]);

                // PARALLELIZATION: Save file and start extraction simultaneously
                const saveFilePromise = fs.writeFile('resultado-consulta.html', content, 'utf-8')
                    .catch(err => console.log('CNPJService: Warning - could not save HTML file:', err.message));

                const extractionPromise = this.startDataExtraction(content, currentUrl, cnpj);

                // Wait for both operations to complete
                const [, extractionResult] = await Promise.all([saveFilePromise, extractionPromise]);
                dadosExtraidos = extractionResult;
                console.log('CNPJService: Current page content saved to resultado-consulta.html');

                // Check current URL to determine extraction strategy
                console.log(`CNPJService: Current URL for extraction: ${currentUrl}`);

                // Check if we're still on the form page (navigation failed)
                if (currentUrl.includes('cnpjreva_solicitacao.asp') || content.includes('Digite o número de CNPJ')) {
                    console.log('CNPJService: ❌ Still on form page - navigation failed!');

                    // Check for captcha errors
                    const pageErrors = await page.evaluate(() => {
                        const errors = [];
                        const errorSelectors = [
                            '#msgErroCaptcha',
                            '#msgErro',
                            '.alert-danger',
                            '.error-message',
                            '[id*="erro"]',
                            '[class*="erro"]'
                        ];

                        errorSelectors.forEach(selector => {
                            const elements = document.querySelectorAll(selector);
                            elements.forEach(element => {
                                if (element && element.style.display !== 'none' && !element.classList.contains('collapse')) {
                                    errors.push({
                                        selector: selector,
                                        text: element.textContent.trim()
                                    });
                                }
                            });
                        });

                        return errors;
                    });

                    if (pageErrors.length > 0) {
                        console.log('CNPJService: ❌ Errors found on page:', JSON.stringify(pageErrors, null, 2));

                        // Enhanced error classification
                        const captchaErrors = pageErrors.filter(e =>
                            e.text.includes('Erro na Consulta') ||
                            e.text.includes('captcha') ||
                            e.text.includes('Esclarecimentos')
                        );

                        // Check if it's specifically "Esclarecimentos adicionais" which can be either captcha or CNPJ issue
                        const esclarecimentosError = pageErrors.find(e =>
                            e.text.includes('Esclarecimentos adicionais')
                        );

                        if (esclarecimentosError) {
                            console.log('CNPJService: 🔍 "Esclarecimentos adicionais" error detected - checking captcha state...');

                            // Check current captcha state to determine if it's a captcha issue
                            const captchaState = await page.evaluate(() => {
                                const textareas = document.querySelectorAll('textarea[name="h-captcha-response"], textarea[id*="h-captcha-response"]');
                                return {
                                    textareasFound: textareas.length,
                                    hasResponse: Array.from(textareas).some(ta => ta.value && ta.value.length > 100),
                                    responses: Array.from(textareas).map(ta => ({
                                        hasValue: ta.value && ta.value.length > 0,
                                        length: ta.value ? ta.value.length : 0
                                    }))
                                };
                            });

                            console.log('CNPJService: Current captcha state:', JSON.stringify(captchaState, null, 2));

                            // 📸 CAPTURAR SCREENSHOT SEMPRE QUE DETECTAR "Esclarecimentos adicionais"
                            try {
                                console.log('CNPJService: 📸 Capturing screenshot for "Esclarecimentos adicionais" error...');
                                await this.captureErrorDebugFiles(
                                    page,
                                    cnpj,
                                    new Error(`Esclarecimentos adicionais detected - Attempt ${attempt} - Captcha has response: ${captchaState.hasResponse} - Textareas found: ${captchaState.textareasFound}`),
                                    'esclarecimentos-error'
                                );
                                console.log('CNPJService: ✅ Screenshot captured successfully for debugging');
                            } catch (screenshotError) {
                                console.log('CNPJService: ⚠️ Could not capture screenshot:', screenshotError.message);
                            }

                            // If captcha has no response, it's likely a captcha issue, not CNPJ not found
                            if (!captchaState.hasResponse) {
                                console.log('CNPJService: 🔄 Captcha appears to be missing - treating as captcha error for retry');
                                // Continue with captcha retry logic below
                            } else {
                                console.log('CNPJService: 🔍 Captcha appears valid - may be genuine CNPJ not found');
                                // Still try retry once more to be sure, but with different approach
                            }
                        }

                        if (captchaErrors.length > 0) {
                            console.log('CNPJService: 🔄 Captcha error detected, attempting comprehensive retry...');

                            // Try a more comprehensive retry approach
                            try {
                                // ADAPTIVE DELAY: Wait for error recovery
                                await this.delay(2000, { pageCondition: 'error_recovery' });

                                // For "Esclarecimentos adicionais" errors, use smart recovery
                                if (esclarecimentosError && attempt < 3) {
                                    console.log('CNPJService: 🔄 Analyzing "Esclarecimentos adicionais" error...');

                                    // Analyze if this error is recoverable
                                    const isRecoverable = await this.analyzeEsclarecimentosError(page, cnpj);

                                    if (isRecoverable) {
                                        console.log('CNPJService: ✅ Error appears recoverable, refreshing page...');

                                        // Navigate back to the form with CNPJ pre-filled
                                        await navigationService.navigateToConsultationPageWithCNPJ(page, cnpj);

                                        // ADAPTIVE DELAY: Wait for page to load completely
                                        await this.smartDelay(page, 'dom_ready', 3000);

                                        // Check if captcha is present
                                        const hasCaptcha = await navigationService.checkIfCaptchaIsPresent(page);
                                        if (hasCaptcha) {
                                            console.log('CNPJService: 🔄 Fresh captcha detected, solving...');
                                            await this.solveCaptcha(page, chaveApi);
                                        }
                                    } else {
                                        console.log('CNPJService: ❌ Error is not recoverable - CNPJ validation issue');
                                        throw new Error(`CNPJ validation error: ${esclarecimentosError.text}`);
                                    }
                                } else {
                                    // Clear any existing captcha responses
                                    await page.evaluate(() => {
                                        const textareas = document.querySelectorAll('textarea[name="h-captcha-response"], textarea[id*="h-captcha-response"]');
                                        textareas.forEach(ta => ta.value = '');
                                    });

                                    // Solve captcha again with fresh approach
                                    console.log('CNPJService: 🔄 Re-solving captcha with fresh approach...');
                                    await this.solveCaptcha(page, chaveApi);
                                }

                                // Wait longer for captcha to be processed
                                await this.delay(2000);

                                // Verify captcha is properly set before retry
                                const preRetryCheck = await page.evaluate(() => {
                                    const textareas = document.querySelectorAll('textarea[name="h-captcha-response"], textarea[id*="h-captcha-response"]');
                                    return Array.from(textareas).map(ta => ({
                                        hasValue: ta.value && ta.value.length > 0,
                                        valueLength: ta.value ? ta.value.length : 0
                                    }));
                                });

                                const hasValidCaptcha = preRetryCheck.some(check => check.hasValue && check.valueLength > 100);
                                if (!hasValidCaptcha) {
                                    throw new Error('Captcha retry failed - no valid captcha response after re-solving');
                                }

                                console.log('CNPJService: ✅ Captcha re-validation successful, attempting form submission...');

                                // Try submitting again with navigation promise
                                const retryNavigationPromise = page.waitForNavigation({
                                    waitUntil: 'domcontentloaded',
                                    timeout: 30000
                                }).catch(err => {
                                    console.log('CNPJService: Retry navigation promise error:', err.message);
                                    return null;
                                });

                                await page.click('button.btn-primary');
                                console.log('CNPJService: Retry form submitted, waiting for navigation...');

                                await retryNavigationPromise;
                                await this.delay(2000);

                                // Check if we succeeded this time
                                const retryUrl = page.url();
                                console.log(`CNPJService: Retry result URL: ${retryUrl}`);

                                if (retryUrl.includes('Cnpjreva_Comprovante.asp')) {
                                    console.log('CNPJService: ✅ Retry successful - reached result page');
                                    // Continue with extraction
                                    const retryContent = await page.content();
                                    await fs.writeFile('resultado-consulta.html', retryContent, 'utf-8').catch(err =>
                                        console.log('CNPJService: Warning - could not save HTML file:', err.message)
                                    );
                                    dadosExtraidos = await extractorService.extrairDadosCNPJ('resultado-consulta.html');
                                    console.log(`CNPJService: Data extraction successful after retry for CNPJ ${cnpj}`);
                                } else {
                                    // Check for errors again with better detection
                                    const retryErrors = await page.evaluate(() => {
                                        const errors = [];
                                        const errorSelectors = ['#msgErroCaptcha', '#msgErro', '.alert-danger', '.error-message'];
                                        errorSelectors.forEach(selector => {
                                            const elements = document.querySelectorAll(selector);
                                            elements.forEach(element => {
                                                if (element && element.style.display !== 'none' && !element.classList.contains('collapse')) {
                                                    const text = element.textContent.trim();
                                                    if (text) {
                                                        errors.push({
                                                            selector: selector,
                                                            text: text,
                                                            isCnpjNotFound: text.includes('Esclarecimentos adicionais') || text.includes('Erro na Consulta')
                                                        });
                                                    }
                                                }
                                            });
                                        });
                                        return errors;
                                    });

                                    // Check if it's a CNPJ not found error
                                    const cnpjNotFoundError = retryErrors.find(e => e.isCnpjNotFound);

                                    if (cnpjNotFoundError) {
                                        // Save error page with screenshot for CNPJ not found (only on final attempt)
                                        if (attempt === 3) {
                                            await this.saveErrorPage(page, cnpj, `CNPJ not found: ${cnpjNotFoundError.text}`);
                                        }
                                        throw new Error(`CNPJ não encontrado na base da Receita Federal (attempt ${attempt}): ${cnpjNotFoundError.text}`);
                                    } else {
                                        // Save retry failure page with screenshot for other errors (only on final attempt)
                                        if (attempt === 3) {
                                            await this.saveErrorPage(page, cnpj, `Retry failed: ${retryErrors.map(e => e.text).join(', ')}`);
                                        }
                                        throw new Error(`Captcha retry failed - still on form page (attempt ${attempt}). Errors: ${retryErrors.map(e => e.text).join(', ')}`);
                                    }
                                }
                            } catch (retryError) {
                                console.log('CNPJService: ❌ Comprehensive retry failed:', retryError.message);
                                throw new Error(`Navigation failed after comprehensive retry: ${retryError.message}`);
                            }
                        } else {
                            throw new Error(`Navigation failed: ${pageErrors.map(e => e.text).join(', ')}`);
                        }
                    } else {
                        throw new Error('Navigation failed: Still on form page, no specific error found');
                    }
                }

                if (currentUrl.includes('Cnpjreva_Comprovante.asp')) {
                    console.log('CNPJService: ✅ On result page, extracting data...');
                    dadosExtraidos = await extractorService.extrairDadosCNPJ('resultado-consulta.html');
                    console.log(`CNPJService: Data extraction successful for CNPJ ${cnpj}`);
                } else if (currentUrl.includes('valida_recaptcha.asp')) {
                    console.log('CNPJService: 🔄 Still on validation page, waiting for redirect...');

                    // Wait for potential redirect
                    try {
                        await page.waitForNavigation({
                            waitUntil: 'domcontentloaded',
                            timeout: 15000
                        });

                        const finalUrl = page.url();
                        console.log(`CNPJService: Final URL after redirect: ${finalUrl}`);

                        if (finalUrl.includes('Cnpjreva_Comprovante.asp')) {
                            const finalContent = await page.content();
                            await fs.writeFile('resultado-consulta.html', finalContent, 'utf-8');
                            dadosExtraidos = await extractorService.extrairDadosCNPJ('resultado-consulta.html');
                            console.log(`CNPJService: Data extraction successful after redirect for CNPJ ${cnpj}`);
                        }
                    } catch (redirectError) {
                        console.log('CNPJService: No redirect occurred, trying to extract from current page...');
                        dadosExtraidos = await extractorService.extrairDadosCNPJ('resultado-consulta.html');
                    }
                } else {
                    console.log('CNPJService: Unexpected page, trying to extract data anyway...');
                    dadosExtraidos = await extractorService.extrairDadosCNPJ('resultado-consulta.html');
                }
            } catch (extractionError) {
                console.error('CNPJService: Error during data extraction:', extractionError.message);
                // Don't throw here, just log and continue
            }
            
            if (dadosExtraidos) {
                const result = this.formatCompleteResult(dadosExtraidos, cnpj);

                // Report successful captcha usage
                if (this.lastCaptchaId) {
                    try {
                        await resilientCaptchaService.reportResult(this.lastCaptchaId, true);
                        console.log('CNPJService: Captcha success reported to API');
                    } catch (reportError) {
                        cnpjLogger.warn({ err: reportError }, 'Failed to report captcha success');
                    }
                    this.lastCaptchaId = null;
                }

                // Cache the result for future requests
                cnpjCache.set(cacheKey, result);

                // PRELOAD OPTIMIZATION: Start background preloading for future requests
                this.startBackgroundPreloading().catch(err =>
                    console.log('CNPJService: Background preloading failed:', err.message)
                );

                const totalTime = Date.now() - startTime;

                // Log performance metrics
                performanceLogger.info({
                    cnpj,
                    executionTime: totalTime,
                    success: true,
                    cached: false,
                    executionId,
                    dataFields: Object.keys(result).length
                }, 'CNPJ consultation completed');

                correlatedLogger.info({
                    cnpj,
                    executionTime: totalTime,
                    dataQuality: result.metadata?.dataQuality || 'unknown',
                    executionId
                }, 'Result cached and consultation completed');

                return result;
            } else {
                console.log(`CNPJService: No data found for CNPJ ${cnpj} on attempt ${attempt}`);
                // On final attempt, return null. On other attempts, throw error to trigger retry
                if (attempt === 3) {
                    return null;
                } else {
                    throw new Error(`No data extracted for CNPJ ${cnpj} - will retry`);
                }
            }
            
        } catch (error) {
            console.error(`CNPJService: Error in consultation attempt ${attempt} for CNPJ ${cnpj}:`, error);

            // Capture screenshot and HTML for any error (only on last attempt to avoid spam)
            try {
                if (page && !page.isClosed() && attempt === 3) {
                    await this.captureErrorDebugFiles(page, cnpj, error, `consultation-error-final-attempt`);
                }
            } catch (debugError) {
                console.log('CNPJService: Could not capture debug files:', debugError.message);
            }

            // Report captcha failure if we have a captcha ID
            if (this.lastCaptchaId) {
                try {
                    await resilientCaptchaService.reportResult(this.lastCaptchaId, false);
                    console.log('CNPJService: Captcha failure reported to API');
                } catch (reportError) {
                    cnpjLogger.warn({ err: reportError }, 'Failed to report captcha failure');
                }
                this.lastCaptchaId = null;
            }

            // For retry logic, we need to throw the error to trigger retry
            // Only return structured error response on final attempt
            if (attempt === 3) {
                // Check if it's a captcha-related error
                if (error.message.includes('captcha') || error.message.includes('Esclarecimentos') || error.message.includes('Navigation failed')) {
                    console.log('CNPJService: 🔄 Captcha-related error detected on final attempt');

                    // Return a structured error response instead of throwing
                    const totalTime = Date.now() - startTime;
                    return {
                        success: false,
                        error: 'CAPTCHA_VALIDATION_FAILED',
                        message: 'A Receita Federal rejeitou a validação do captcha após 3 tentativas. Isso pode ser devido a mudanças no sistema ou alta demanda.',
                        details: {
                            originalError: error.message,
                            cnpj: cnpj,
                            executionTime: totalTime,
                            timestamp: new Date().toISOString(),
                            attemptsUsed: 3,
                            suggestions: [
                                'Tente novamente em alguns minutos',
                                'Verifique se o CNPJ está correto',
                                'O sistema da Receita Federal pode estar temporariamente indisponível'
                            ]
                        },
                        metadata: {
                            captchaSolved: true,
                            navigationAttempted: true,
                            retryAttempted: true,
                            finalAttempt: true,
                            version: '2.0.0'
                        }
                    };
                }

                // Only return null on final attempt after all retries
                if (attempt === 3 && (error.message.includes('timeout') || error.message.includes('não encontrado') || error.message.includes('No data extracted'))) {
                    console.log(`CNPJService: CNPJ not found after all 3 attempts: ${error.message}`);
                    return null;
                }
            }

            // Throw error to trigger retry (for non-final attempts) or final failure
            throw new Error(`Failed to consult CNPJ (attempt ${attempt}): ${error.message}`);
        } finally {
            if (browser) {
                console.log('CNPJService: Returning browser to pool...');
                try {
                    await browserPool.returnBrowser(browser);
                } catch (closeError) {
                    console.error('CNPJService: Error returning browser to pool:', closeError.message);
                }
            }
        }
    }

    // Browser launch method removed - now using browser pool









    /**
     * Solve hCaptcha with improved detection
     */
    async solveCaptcha(page, apiKey) {
        console.log('CNPJService: Starting hCaptcha resolution...');
        console.log(`CNPJService: API Key provided: ${apiKey ? 'Yes' : 'No'}`);
        console.log(`CNPJService: Current page URL: ${page.url()}`);

        if (apiKey) {
            try {
                console.log('CNPJService: Waiting for hCaptcha element...');
                // Wait for hCaptcha to load (optimized)
                await page.waitForSelector('.h-captcha', { timeout: 8000 });
                console.log('CNPJService: hCaptcha element found, waiting for complete loading...');
                // ADAPTIVE DELAY: Wait for captcha to be fully loaded
                await this.smartDelay(page, 'captcha_ready', 2000);

                console.log('CNPJService: Extracting site key from page...');
                
                // Extract site key with multiple methods
                const siteKey = await page.evaluate(() => {
                    // Method 1: data-sitekey attribute
                    const hcaptchaDiv = document.querySelector('.h-captcha');
                    if (hcaptchaDiv && hcaptchaDiv.getAttribute('data-sitekey')) {
                        console.log('Site key found via data-sitekey attribute');
                        return hcaptchaDiv.getAttribute('data-sitekey');
                    }
                    
                    // Method 2: Search in scripts
                    const scriptTags = Array.from(document.querySelectorAll('script[src*="hcaptcha.com"]'));
                    for (const script of scriptTags) {
                        const src = script.src;
                        const match = src.match(/sitekey=([^&]+)/);
                        if (match) {
                            console.log('Site key found in script src');
                            return match[1];
                        }
                    }
                    
                    // Method 3: Search in HTML content
                    const htmlContent = document.documentElement.innerHTML;
                    const sitekeyMatch = htmlContent.match(/data-sitekey="([^"]+)"/);
                    if (sitekeyMatch) {
                        console.log('Site key found in HTML content');
                        return sitekeyMatch[1];
                    }
                    
                    console.log('Site key not found');
                    return null;
                });
                
                if (siteKey) {
                    console.log(`CNPJService: hCaptcha site key found: ${siteKey}`);
                    console.log('CNPJService: Solving hCaptcha using API...');
                    console.log(`CNPJService: Page URL for captcha: ${page.url()}`);

                    // Solve hCaptcha using resilient service
                    console.log('CNPJService: Using resilient captcha service...');
                    const captchaResult = await resilientCaptchaService.solveHCaptcha(siteKey, page.url());
                    console.log(`CNPJService: Captcha solved successfully`);
                    console.log(`CNPJService: Token received: ${captchaResult.token ? captchaResult.token.substring(0, 50) + '...' : 'null'}`);
                    console.log(`CNPJService: User Agent: ${captchaResult.useragent || 'not provided'}`);

                    // Set user agent if provided
                    if (captchaResult.useragent) {
                        await page.setUserAgent(captchaResult.useragent);
                        console.log('CNPJService: User agent updated');
                    }

                    // Inject response with SUPER ROBUST method
                    const injectionResult = await page.evaluate((response) => {
                        console.log('Injecting hCaptcha response:', response.substring(0, 50) + '...');

                        // Method 1: Find all textareas with h-captcha-response (ENHANCED)
                        const textareas = document.querySelectorAll('textarea[name="h-captcha-response"], textarea[id*="h-captcha-response"], textarea[id*="hcaptcha"]');
                        console.log('Found textareas:', textareas.length);

                        let injectedCount = 0;
                        textareas.forEach((textarea, index) => {
                            // SUPER ROBUST injection - multiple methods

                            // Method A: Direct value assignment
                            textarea.value = response;

                            // Method B: Property assignment
                            textarea.setAttribute('value', response);

                            // Method C: Force property descriptor
                            Object.defineProperty(textarea, 'value', {
                                value: response,
                                writable: true,
                                configurable: true
                            });

                            // Method D: Direct property set
                            textarea.defaultValue = response;

                            // Method E: Force via innerHTML (for some edge cases)
                            textarea.innerHTML = response;

                            // Method F: Set data attribute as backup
                            textarea.setAttribute('data-captcha-response', response);

                            // Trigger ALL possible events
                            const events = ['input', 'change', 'blur', 'focus', 'keyup', 'keydown', 'paste'];
                            events.forEach(eventType => {
                                textarea.dispatchEvent(new Event(eventType, { bubbles: true, cancelable: true }));
                            });

                            // Custom event for hCaptcha
                            textarea.dispatchEvent(new CustomEvent('hcaptcha-response', {
                                detail: { response: response },
                                bubbles: true
                            }));

                            console.log(`hCaptcha response SUPER-injected in textarea ${index + 1}`);
                            console.log(`Textarea ${index + 1} value after injection:`, textarea.value.substring(0, 50) + '...');
                            console.log(`Textarea ${index + 1} attribute value:`, textarea.getAttribute('value')?.substring(0, 50) + '...');
                            injectedCount++;
                        });

                        // Method 2: Try to find and set via hCaptcha widget
                        if (window.hcaptcha && window.hcaptcha.getResponse) {
                            try {
                                // Get all widget IDs
                                const widgets = document.querySelectorAll('.h-captcha');
                                widgets.forEach((widget) => {
                                    const widgetId = widget.getAttribute('data-hcaptcha-widget-id');
                                    if (widgetId) {
                                        console.log(`Setting response for widget ${widgetId}`);
                                        // Force set the response
                                        const responseTextarea = document.querySelector(`textarea[data-hcaptcha-widget-id="${widgetId}"]`);
                                        if (responseTextarea) {
                                            responseTextarea.value = response;
                                            responseTextarea.dispatchEvent(new Event('change', { bubbles: true }));
                                            injectedCount++;
                                        }
                                    }
                                });
                            } catch (e) {
                                console.log('Error setting hCaptcha widget response:', e);
                            }
                        }

                        // Method 3: Direct DOM manipulation
                        try {
                            // Force set all possible hCaptcha response fields
                            const allPossibleFields = document.querySelectorAll('textarea, input[type="hidden"]');
                            allPossibleFields.forEach((field) => {
                                if (field.name && field.name.includes('captcha') ||
                                    field.id && field.id.includes('captcha') ||
                                    field.className && field.className.includes('captcha')) {
                                    field.value = response;
                                    console.log(`Set captcha response in field: ${field.name || field.id || field.className}`);
                                    injectedCount++;
                                }
                            });
                        } catch (e) {
                            console.log('Error in direct DOM manipulation:', e);
                        }

                        return {
                            textareasFound: textareas.length,
                            injectedCount: injectedCount,
                            finalValues: Array.from(textareas).map(ta => ({
                                name: ta.name,
                                id: ta.id,
                                value: ta.value ? ta.value.substring(0, 50) + '...' : 'empty'
                            }))
                        };
                    }, captchaResult.token);

                    console.log('CNPJService: Injection result:', JSON.stringify(injectionResult, null, 2));
                    
                    console.log('CNPJService: hCaptcha solved and injected successfully');

                    // Enhanced validation to ensure captcha is properly set
                    console.log('CNPJService: Performing enhanced captcha validation...');
                    let validationAttempts = 0;
                    const maxValidationAttempts = 5;
                    let captchaValid = false;

                    while (!captchaValid && validationAttempts < maxValidationAttempts) {
                        validationAttempts++;

                        const validationResult = await page.evaluate(() => {
                            const textareas = document.querySelectorAll('textarea[name="h-captcha-response"], textarea[id*="h-captcha-response"], textarea[id*="hcaptcha"]');
                            const results = Array.from(textareas).map(ta => ({
                                name: ta.name,
                                id: ta.id,
                                hasValue: ta.value && ta.value.length > 0,
                                valueLength: ta.value ? ta.value.length : 0,
                                value: ta.value ? ta.value.substring(0, 50) + '...' : 'empty',
                                attributeValue: ta.getAttribute('value') ? ta.getAttribute('value').substring(0, 50) + '...' : 'empty',
                                dataAttribute: ta.getAttribute('data-captcha-response') ? ta.getAttribute('data-captcha-response').substring(0, 50) + '...' : 'empty'
                            }));

                            // SUPER validation - check multiple sources
                            const hasValidResponse = results.some(r =>
                                (r.hasValue && r.valueLength > 100) ||
                                (r.attributeValue !== 'empty' && r.attributeValue.length > 50) ||
                                (r.dataAttribute !== 'empty' && r.dataAttribute.length > 50)
                            );

                            return {
                                textareasFound: textareas.length,
                                results: results,
                                hasValidResponse: hasValidResponse
                            };
                        });

                        console.log(`CNPJService: Validation attempt ${validationAttempts}:`, JSON.stringify(validationResult, null, 2));

                        if (validationResult.hasValidResponse) {
                            captchaValid = true;
                            console.log('CNPJService: ✅ Captcha validation successful');
                        } else {
                            console.log(`CNPJService: ⚠️ Captcha validation failed, attempt ${validationAttempts}/${maxValidationAttempts}`);

                            if (validationAttempts < maxValidationAttempts) {
                                // SUPER Re-inject captcha token with all methods
                                console.log('CNPJService: SUPER Re-injecting captcha token...');
                                await page.evaluate((token) => {
                                    const textareas = document.querySelectorAll('textarea[name="h-captcha-response"], textarea[id*="h-captcha-response"], textarea[id*="hcaptcha"]');
                                    let injected = 0;
                                    textareas.forEach(ta => {
                                        // Use all injection methods
                                        ta.value = token;
                                        ta.setAttribute('value', token);
                                        ta.setAttribute('data-captcha-response', token);
                                        ta.defaultValue = token;
                                        ta.innerHTML = token;

                                        // Force property descriptor
                                        try {
                                            Object.defineProperty(ta, 'value', {
                                                value: token,
                                                writable: true,
                                                configurable: true
                                            });
                                        } catch (e) {
                                            console.log('Property descriptor failed:', e);
                                        }

                                        // Trigger all events
                                        ['input', 'change', 'blur', 'focus', 'keyup', 'paste'].forEach(eventType => {
                                            ta.dispatchEvent(new Event(eventType, { bubbles: true, cancelable: true }));
                                        });

                                        injected++;
                                        console.log(`Re-injected in textarea ${ta.id || ta.name}, value length: ${ta.value.length}`);
                                    });
                                    return injected;
                                }, captchaResult.token);

                                await this.delay(3000); // Increased wait time for better reliability
                            }
                        }
                    }

                    if (!captchaValid) {
                        throw new Error('Failed to validate captcha injection after multiple attempts');
                    }

                    console.log('CNPJService: Waiting for captcha processing...');
                    await this.delay(1500); // Increased back to 1500ms for better reliability

                    // Verify injection was successful
                    const verificationResult = await page.evaluate(() => {
                        const textareas = document.querySelectorAll('textarea[name="h-captcha-response"], textarea[id*="h-captcha-response"]');
                        const results = [];
                        textareas.forEach((textarea, index) => {
                            results.push({
                                index: index,
                                hasValue: textarea.value && textarea.value.length > 0,
                                valueLength: textarea.value ? textarea.value.length : 0,
                                name: textarea.name,
                                id: textarea.id
                            });
                        });
                        return results;
                    });
                    console.log('CNPJService: Captcha injection verification:', JSON.stringify(verificationResult, null, 2));

                    // Store captcha ID for later reporting
                    this.lastCaptchaId = captchaResult.captchaId;

                } else {
                    cnpjLogger.error('Could not find hCaptcha site key on page');
                    throw new Error('hCaptcha site key not found - cannot proceed with API resolution');
                }
            } catch (error) {
                cnpjLogger.error({ err: error }, 'Failed to solve captcha via resilient API');

                // Get health report from resilient service
                const healthReport = resilientCaptchaService.getHealthReport();
                cnpjLogger.info({ healthReport }, 'Captcha service health status');

                // Capture screenshot and HTML of captcha error since we have page access here
                try {
                    if (page && !page.isClosed()) {
                        await this.captureErrorDebugFiles(page, 'captcha-error', error, 'captcha-error');
                    }
                } catch (captureError) {
                    cnpjLogger.warn({ err: captureError }, 'Failed to capture captcha error debug files');
                }

                // Check if it's a circuit breaker issue
                if (error.message.includes('circuit breaker')) {
                    throw new Error(`Captcha service temporarily unavailable: ${error.message}`);
                }

                throw new Error(`Resilient captcha API failed: ${error.message}`);
            }
        } else {
            console.log('CNPJService: No API key provided, using manual captcha resolution');
            await this.handleManualCaptcha(page);
        }
    }

    /**
     * Handle manual captcha resolution
     */
    async handleManualCaptcha(page) {
        try {
            // Wait for hCaptcha iframe to load
            await page.waitForSelector('.h-captcha iframe', { timeout: 10000 });
            await this.delay(2000);
            
            console.log('CNPJService: Attempting to click hCaptcha checkbox...');
            
            // Try to click checkbox in iframe
            const frames = await page.frames();
            let checkboxClicked = false;
            
            for (const frame of frames) {
                try {
                    const checkbox = await frame.$('#checkbox');
                    if (checkbox) {
                        await frame.click('#checkbox');
                        console.log('CNPJService: hCaptcha checkbox clicked successfully');
                        checkboxClicked = true;
                        break;
                    }
                } catch (e) {
                    // Continue trying other frames
                }
            }
            
            if (!checkboxClicked) {
                console.log('CNPJService: Could not find hCaptcha checkbox');
            }
            
            // Wait for manual resolution (with timeout)
            console.log('CNPJService: Please solve the captcha manually. Waiting 60 seconds...');
            await this.delay(60000); // Wait 60 seconds for manual resolution
            
        } catch (error) {
            console.log(`CNPJService: Error in manual captcha handling: ${error.message}`);
        }
    }

    /**
     * Submit form and wait for results - FIXED VERSION
     */
    async submitFormAndWaitForResults(page) {
        console.log('CNPJService: Starting form submission process...');

        try {
            console.log('CNPJService: Waiting for submit button...');
            // Wait for submit button
            await page.waitForSelector('button.btn-primary', { timeout: 10000 });
            console.log('CNPJService: Submit button found');

            // Detailed form state verification
            const formState = await page.evaluate(() => {
                const cnpjInput = document.querySelector('input[name="cnpj"]');
                const captchaTextareas = document.querySelectorAll('textarea[name="h-captcha-response"], textarea[id*="h-captcha-response"]');
                const submitButton = document.querySelector('button.btn-primary');
                const hcaptchaDiv = document.querySelector('.h-captcha');

                return {
                    cnpjValue: cnpjInput ? cnpjInput.value : 'not found',
                    captchaCount: captchaTextareas.length,
                    captchaValues: Array.from(captchaTextareas).map(ta => ({
                        name: ta.name,
                        id: ta.id,
                        hasValue: ta.value && ta.value.length > 0,
                        valueLength: ta.value ? ta.value.length : 0,
                        value: ta.value ? ta.value.substring(0, 50) + '...' : 'empty'
                    })),
                    submitButtonExists: !!submitButton,
                    submitButtonText: submitButton ? submitButton.textContent.trim() : 'not found',
                    submitButtonEnabled: submitButton ? !submitButton.disabled : false,
                    hcaptchaExists: !!hcaptchaDiv,
                    hcaptchaSiteKey: hcaptchaDiv ? hcaptchaDiv.getAttribute('data-sitekey') : 'not found'
                };
            });
            console.log('CNPJService: Detailed form state:', JSON.stringify(formState, null, 2));

            // Verify captcha response is set before submitting
            const captchaVerified = formState.captchaValues.some(cv => cv.hasValue);

            if (!captchaVerified) {
                console.log('CNPJService: ❌ CRITICAL - No captcha response found!');
                console.log('CNPJService: This will likely cause submission to fail');

                // Try to re-inject captcha if we have a recent solution
                console.log('CNPJService: Attempting to re-solve captcha...');

                // Wait a bit before retrying
                await this.delay(2000);

                await this.solveCaptcha(page, chaveApi);

                // Re-check after re-injection with more thorough validation
                const recheckState = await page.evaluate(() => {
                    const captchaTextareas = document.querySelectorAll('textarea[name="h-captcha-response"], textarea[id*="h-captcha-response"]');
                    const results = Array.from(captchaTextareas).map(ta => ({
                        name: ta.name,
                        id: ta.id,
                        hasValue: ta.value && ta.value.length > 0,
                        valueLength: ta.value ? ta.value.length : 0,
                        value: ta.value ? ta.value.substring(0, 50) + '...' : ''
                    }));

                    // Also check if hCaptcha widget shows as solved
                    const hcaptchaWidget = document.querySelector('.h-captcha');
                    const isSolved = hcaptchaWidget ? hcaptchaWidget.getAttribute('data-hcaptcha-response') : null;

                    return {
                        textareas: results,
                        widgetSolved: !!isSolved
                    };
                });

                const recheckVerified = recheckState.textareas.some(cv => cv.hasValue) || recheckState.widgetSolved;
                if (!recheckVerified) {
                    console.log('CNPJService: ❌ Captcha re-injection failed:', JSON.stringify(recheckState, null, 2));
                    throw new Error('Captcha verification failed after retry - cannot proceed with submission');
                }
                console.log('CNPJService: ✅ Captcha re-injection successful:', JSON.stringify(recheckState.textareas, null, 2));
            } else {
                console.log('CNPJService: ✅ Captcha response verified successfully');
                console.log(`CNPJService: Found ${formState.captchaValues.filter(cv => cv.hasValue).length} captcha responses`);
            }

            // Get current URL before clicking
            const initialUrl = page.url();
            console.log(`CNPJService: Initial URL: ${initialUrl}`);

            // ADAPTIVE DELAY: Wait for form to be ready for submission
            console.log('CNPJService: Waiting for form to be ready for submission...');
            await this.smartDelay(page, 'form_ready', 2000);

            // Final check before clicking
            console.log('CNPJService: Performing final check before clicking submit...');
            const finalCheck = await page.evaluate(() => {
                const button = document.querySelector('button.btn-primary');
                return {
                    buttonExists: !!button,
                    buttonVisible: button ? button.offsetParent !== null : false,
                    buttonEnabled: button ? !button.disabled : false,
                    buttonText: button ? button.textContent.trim() : 'not found'
                };
            });
            console.log('CNPJService: Final button check:', JSON.stringify(finalCheck, null, 2));

            // Set up navigation promise before clicking
            console.log('CNPJService: Setting up navigation promise...');
            const navigationPromise = page.waitForNavigation({
                waitUntil: 'domcontentloaded',
                timeout: 30000 // Reduced from 60000ms to 30000ms
            }).catch(err => {
                console.log('CNPJService: Navigation promise error (this might be expected):', err.message);
                return null; // Don't throw, just return null
            });

            // Click the button naturally like in the recording
            console.log('CNPJService: Clicking submit button naturally...');

            // Try to get button for natural click
            const submitButton = await page.$('button.btn-primary');
            if (submitButton) {
                const buttonBox = await submitButton.boundingBox();
                if (buttonBox) {
                    // Click with natural offset similar to recording (65, 6.546875)
                    const clickX = buttonBox.x + 65;
                    const clickY = buttonBox.y + 6.5;
                    await page.mouse.click(clickX, clickY);
                    console.log(`CNPJService: Clicked at natural position (${clickX}, ${clickY})`);
                } else {
                    // Fallback to regular click
                    await page.click('button.btn-primary');
                    console.log('CNPJService: Used fallback click method');
                }
            } else {
                await page.click('button.btn-primary');
                console.log('CNPJService: Used selector click method');
            }

            console.log('CNPJService: Submit button clicked, waiting for navigation...');

            // Wait for navigation to complete
            console.log('CNPJService: Waiting for navigation promise...');
            const navigationResult = await navigationPromise;

            if (navigationResult) {
                console.log('CNPJService: ✅ Navigation completed successfully!');
            } else {
                console.log('CNPJService: ⚠️ Navigation promise failed, checking manually...');
            }

            // Give some time for the page to load completely (optimized)
            await this.delay(1000); // Further reduced from 1500ms to 1000ms

            // Check current URL and page state
            let currentUrl;
            let pageTitle;
            try {
                currentUrl = page.url();
                pageTitle = await page.title();
                console.log(`CNPJService: Current URL after navigation: ${currentUrl}`);
                console.log(`CNPJService: Current page title: ${pageTitle}`);
            } catch (error) {
                console.log('CNPJService: Error getting page info after navigation:', error.message);
                // Try to get URL without title
                try {
                    currentUrl = page.url();
                    console.log(`CNPJService: Current URL: ${currentUrl}`);
                } catch (urlError) {
                    console.log('CNPJService: Cannot get URL, page might be navigating...');
                    currentUrl = 'unknown';
                }
            }

            // Check if we reached the expected result page
            if (currentUrl && currentUrl.includes('Cnpjreva_Comprovante.asp')) {
                console.log('CNPJService: ✅ Successfully reached the CNPJ result page!');
            } else if (currentUrl && currentUrl.includes('valida_recaptcha.asp')) {
                console.log('CNPJService: 🔄 On captcha validation page, waiting for redirect...');

                // Wait for redirect to result page
                try {
                    await page.waitForNavigation({
                        waitUntil: 'domcontentloaded',
                        timeout: 30000
                    });

                    const finalUrl = page.url();
                    console.log(`CNPJService: Final URL after redirect: ${finalUrl}`);

                    if (finalUrl.includes('Cnpjreva_Comprovante.asp')) {
                        console.log('CNPJService: ✅ Successfully reached the CNPJ result page after redirect!');
                    }
                } catch (redirectError) {
                    console.log('CNPJService: ⚠️ Redirect timeout, but continuing...', redirectError.message);
                }
            } else if (currentUrl !== initialUrl) {
                console.log('CNPJService: ✅ Navigation completed - reached a different page');
            } else if (currentUrl && currentUrl.includes('cnpjreva_solicitacao.asp')) {
                console.log('CNPJService: ❌ Still on form page - submission failed!');

                // Check for specific captcha errors
                try {
                    const captchaErrors = await page.evaluate(() => {
                        const errors = [];

                        // Check for visible error messages
                        const errorElements = document.querySelectorAll('[id*="erro"], [class*="erro"], .alert-danger, .error');
                        errorElements.forEach(element => {
                            if (element.offsetParent !== null && element.textContent.trim()) {
                                errors.push(element.textContent.trim());
                            }
                        });

                        // Check if captcha is still visible (indicating failure)
                        const captchaElement = document.querySelector('.h-captcha');
                        if (captchaElement && captchaElement.offsetParent !== null) {
                            errors.push('Captcha still visible - may indicate validation failure');
                        }

                        return errors;
                    });

                    if (captchaErrors.length > 0) {
                        console.log('CNPJService: ❌ Captcha/Form errors detected:', captchaErrors);
                        throw new Error(`Form submission failed: ${captchaErrors.join(', ')}`);
                    } else {
                        throw new Error('Form submission failed: Still on form page with no specific error');
                    }
                } catch (errorCheckError) {
                    console.log('CNPJService: Could not check for form errors:', errorCheckError.message);
                    throw new Error('Form submission failed: Still on form page');
                }
            } else {
                console.log('CNPJService: ⚠️ Still on the same page, checking for errors...');

                // Check for errors on the current page
                try {
                    const pageErrors = await page.evaluate(() => {
                        const errors = [];
                        const errorSelectors = [
                            '#msgErroCaptcha',
                            '#msgErro',
                            '.alert-danger',
                            '.error-message'
                        ];

                        errorSelectors.forEach(selector => {
                            const element = document.querySelector(selector);
                            if (element && !element.classList.contains('collapse') && element.style.display !== 'none') {
                                errors.push({
                                    selector: selector,
                                    text: element.textContent.trim()
                                });
                            }
                        });

                        return errors;
                    });

                    if (pageErrors.length > 0) {
                        console.log('CNPJService: ❌ Errors found on page:', JSON.stringify(pageErrors, null, 2));
                    }
                } catch (errorCheckError) {
                    console.log('CNPJService: Could not check for page errors:', errorCheckError.message);
                }
            }
            
            // Wait for page to stabilize (optimized)
            await this.delay(2000); // Reduced from 5000ms to 2000ms

            // Check current URL
            const finalCurrentUrl = page.url();
            console.log(`CNPJService: Current URL: ${finalCurrentUrl}`);
            
            if (finalCurrentUrl.includes('Cnpjreva_Comprovante.asp')) {
                console.log('CNPJService: Successfully reached results page');
                
                // Wait for results table to load
                try {
                    await page.waitForSelector('table', { timeout: 15000 });
                    console.log('CNPJService: Results table found');
                    return true;
                } catch (tableError) {
                    console.log('CNPJService: Results table not found, but on correct page');
                    return true; // Still try to extract data
                }
                
            } else if (currentUrl.includes('cnpjreva_solicitacao.asp')) {
                console.log('CNPJService: Still on consultation page, checking for errors...');
                
                // Check for error messages with comprehensive analysis
                const hasErrors = await page.evaluate(() => {
                    const errors = [];
                    const analysis = {
                        url: window.location.href,
                        title: document.title,
                        isFormPage: window.location.href.includes('Solicitacao.asp'),
                        isResultPage: window.location.href.includes('Comprovante.asp')
                    };

                    // Check captcha error (visible)
                    const errorDiv = document.querySelector('#msgErroCaptcha');
                    if (errorDiv && errorDiv.offsetParent !== null) {
                        const text = errorDiv.textContent.trim();
                        if (text) {
                            errors.push({ type: 'captcha', message: text, element: 'msgErroCaptcha' });
                        }
                    }

                    // Check general error (visible)
                    const msgErro = document.querySelector('#msgErro');
                    if (msgErro && !msgErro.classList.contains('collapse') && msgErro.offsetParent !== null) {
                        const text = msgErro.textContent.trim();
                        if (text) {
                            errors.push({ type: 'general', message: text, element: 'msgErro' });
                        }
                    }

                    // Specific patterns for different error types
                    const errorPatterns = {
                        CNPJ_NOT_FOUND: [
                            'Esclarecimentos adicionais',
                            'Erro na Consulta',
                            'CNPJ não encontrado',
                            'não foi possível obter'
                        ],
                        CAPTCHA_ERROR: [
                            'captcha inválido',
                            'captcha incorreto',
                            'verificação falhou'
                        ],
                        FORM_ERROR: [
                            'Campos não preenchidos',
                            'CNPJ Inválido',
                            'dados obrigatórios'
                        ]
                    };

                    // Classify error type
                    let errorType = 'OTHER_ERROR';
                    let primaryError = null;

                    if (errors.length > 0) {
                        primaryError = errors[0];

                        // Check each pattern
                        for (const [type, patterns] of Object.entries(errorPatterns)) {
                            if (patterns.some(pattern =>
                                primaryError.message.toLowerCase().includes(pattern.toLowerCase())
                            )) {
                                errorType = type;
                                break;
                            }
                        }

                        return {
                            error: true,
                            type: errorType,
                            message: primaryError.message,
                            allErrors: errors,
                            analysis: analysis,
                            isStillOnFormPage: analysis.isFormPage
                        };
                    }

                    // No visible errors found
                    return {
                        error: false,
                        analysis: analysis,
                        isStillOnFormPage: analysis.isFormPage
                    };
                });
                
                if (hasErrors.error) {
                    // Save error page for analysis
                    await this.saveErrorPage(page, cnpj, `${hasErrors.type}: ${hasErrors.message}`);

                    // Log detailed error information
                    cnpjLogger.warn({
                        cnpj,
                        errorType: hasErrors.type,
                        message: hasErrors.message,
                        isStillOnFormPage: hasErrors.isStillOnFormPage,
                        url: hasErrors.analysis?.url,
                        allErrors: hasErrors.allErrors?.length || 0
                    }, 'Error detected during CNPJ consultation');

                    // Handle different error types appropriately
                    switch (hasErrors.type) {
                        case 'CNPJ_NOT_FOUND':
                            throw new Error(`CNPJ não encontrado na base da Receita Federal (attempt ${attempt}): ${hasErrors.message}`);

                        case 'CAPTCHA_ERROR':
                            throw new Error(`Erro de captcha: ${hasErrors.message}`);

                        case 'FORM_ERROR':
                            throw new Error(`Erro no formulário: ${hasErrors.message}`);

                        default:
                            throw new Error(`Erro na consulta: ${hasErrors.message}`);
                    }
                }
                
                return false;
            } else {
                console.log('CNPJService: Unexpected page reached');
                return false;
            }
            
        } catch (error) {
            console.error(`CNPJService: Error during form submission: ${error.message}`);

            // If it's a navigation error, it might actually be successful
            if (error.message.includes('Execution context was destroyed') ||
                error.message.includes('Navigation timeout') ||
                error.message.includes('most likely because of a navigation')) {
                console.log('CNPJService: Navigation error detected, checking if page actually navigated...');

                try {
                    // Wait a bit more for navigation to complete
                    await this.delay(3000);

                    const currentUrl = page.url();
                    console.log(`CNPJService: URL after navigation error: ${currentUrl}`);

                    if (currentUrl.includes('Cnpjreva_Comprovante.asp') ||
                        currentUrl.includes('valida_recaptcha.asp')) {
                        console.log('CNPJService: ✅ Navigation was actually successful despite error!');
                        // Continue with the process instead of throwing
                        return true;
                    }
                } catch (checkError) {
                    console.log('CNPJService: Could not check navigation status:', checkError.message);
                }
            }

            // Save error page and screenshot for detailed analysis
            try {
                if (page && !page.isClosed()) {
                    const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
                    const content = await page.content();
                    const url = page.url();
                    const title = await page.title();

                    // Create debug info
                    const debugInfo = `
<!--
DEBUG INFO:
Timestamp: ${timestamp}
CNPJ: ${cnpj}
URL: ${url}
Title: ${title}
Error: ${error.message}
Stack: ${error.stack}
-->
${content}`;

                    // Save HTML
                    const htmlFilename = `debug/error-${cnpj}-${timestamp}.html`;
                    await fs.mkdir('debug', { recursive: true });
                    await fs.writeFile(htmlFilename, debugInfo, 'utf-8');

                    // Capture screenshot
                    let screenshotFilename = null;
                    try {
                        screenshotFilename = `debug/screenshot-${cnpj}-${timestamp}.png`;
                        await page.screenshot({
                            path: screenshotFilename,
                            fullPage: true,
                            type: 'png'
                        });
                    } catch (screenshotError) {
                        cnpjLogger.warn({ err: screenshotError }, 'Failed to capture error screenshot');
                    }

                    cnpjLogger.info({
                        htmlFile: htmlFilename,
                        screenshotFile: screenshotFilename,
                        url,
                        title
                    }, 'Error page and screenshot saved for analysis');

                    console.log(`CNPJService: 🔍 Error debug files saved:`);
                    console.log(`  📄 HTML: ${htmlFilename}`);
                    if (screenshotFilename) {
                        console.log(`  📸 Screenshot: ${screenshotFilename}`);
                    }
                }
            } catch (saveError) {
                cnpjLogger.error({ err: saveError }, 'Could not save error page/screenshot');
            }

            throw error;
        }
    }

    /**
     * Analyze URL to understand page type and parameters
     */
    analyzeUrl(url) {
        const analysis = {
            url: url,
            isFormPage: false,
            isResultPage: false,
            isErrorPage: false,
            hasParameters: false,
            parameters: {},
            pageType: 'unknown'
        };

        try {
            const urlObj = new URL(url);

            // Check page type by filename
            const pathname = urlObj.pathname.toLowerCase();
            if (pathname.includes('solicitacao')) {
                analysis.isFormPage = true;
                analysis.pageType = 'form';
            } else if (pathname.includes('comprovante')) {
                analysis.isResultPage = true;
                analysis.pageType = 'result';
            } else if (pathname.includes('erro')) {
                analysis.isErrorPage = true;
                analysis.pageType = 'error';
            }

            // Extract parameters
            if (urlObj.search) {
                analysis.hasParameters = true;
                urlObj.searchParams.forEach((value, key) => {
                    analysis.parameters[key] = value;
                });
            }

            // Special analysis for CNPJ pages
            if (analysis.parameters.cnpj) {
                analysis.cnpjInUrl = analysis.parameters.cnpj;
                analysis.cnpjFormatted = this.formatCNPJ(analysis.parameters.cnpj);
            }

        } catch (error) {
            analysis.error = error.message;
        }

        return analysis;
    }

    /**
     * Format CNPJ for display
     */
    formatCNPJ(cnpj) {
        const clean = cnpj.replace(/\D/g, '');
        if (clean.length === 14) {
            return clean.replace(/(\d{2})(\d{3})(\d{3})(\d{4})(\d{2})/, '$1.$2.$3/$4-$5');
        }
        return cnpj;
    }

    /**
     * Capture error debug files (screenshot + HTML)
     */
    async captureErrorDebugFiles(page, cnpj, error, errorType = 'error') {
        try {
            const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
            const url = page.url();
            const title = await page.title();

            // Capture screenshot
            const screenshotPath = `debug/${errorType}-screenshot-${cnpj}-${timestamp}.png`;
            await fs.mkdir('debug', { recursive: true });
            await page.screenshot({
                path: screenshotPath,
                fullPage: true,
                type: 'png'
            });

            // Save HTML
            const htmlPath = `debug/${errorType}-${cnpj}-${timestamp}.html`;
            const content = await page.content();
            const debugInfo = `
<!--
DEBUG INFO - ${errorType.toUpperCase()}:
Timestamp: ${timestamp}
CNPJ: ${cnpj}
URL: ${url}
Title: ${title}
Error: ${error.message}
Stack: ${error.stack}
-->
${content}`;

            await fs.writeFile(htmlPath, debugInfo, 'utf-8');

            console.log(`CNPJService: 🔍 ${errorType} debug files captured:`);
            console.log(`  📸 Screenshot: ${screenshotPath}`);
            console.log(`  📄 HTML: ${htmlPath}`);

            cnpjLogger.info({
                errorType,
                screenshotFile: screenshotPath,
                htmlFile: htmlPath,
                url,
                title,
                cnpj
            }, 'Error debug files captured');

        } catch (captureError) {
            console.log(`CNPJService: ⚠️ Could not capture ${errorType} debug files:`, captureError.message);
            cnpjLogger.warn({ err: captureError, errorType }, 'Failed to capture error debug files');
        }
    }

    /**
     * Save error page for debugging
     */
    async saveErrorPage(page, cnpj, errorMessage) {
        try {
            if (!page || page.isClosed()) return;

            const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
            const content = await page.content();
            const url = page.url();
            const title = await page.title();

            // Get comprehensive page analysis
            const pageAnalysis = await page.evaluate(() => {
                const analysis = {
                    errors: [],
                    pageElements: {},
                    formState: {},
                    captchaState: {},
                    pageContent: {
                        title: document.title,
                        url: window.location.href,
                        hasTable: !!document.querySelector('table'),
                        hasForm: !!document.querySelector('form'),
                        hasCaptcha: !!document.querySelector('.h-captcha')
                    }
                };

                // Check for errors
                const errorSelectors = [
                    '#msgErroCaptcha',
                    '#msgErro',
                    '.alert-danger',
                    '.error-message',
                    '[class*="erro"]',
                    '.alert-warning',
                    '.alert-info'
                ];

                errorSelectors.forEach(selector => {
                    const elements = document.querySelectorAll(selector);
                    elements.forEach(element => {
                        if (element && element.textContent.trim()) {
                            analysis.errors.push({
                                selector: selector,
                                text: element.textContent.trim(),
                                visible: element.offsetParent !== null,
                                className: element.className,
                                id: element.id
                            });
                        }
                    });
                });

                // Check form state
                const cnpjField = document.querySelector('#cnpj');
                if (cnpjField) {
                    analysis.formState.cnpj = {
                        value: cnpjField.value,
                        disabled: cnpjField.disabled,
                        readonly: cnpjField.readOnly
                    };
                }

                // Check captcha state
                const captchaElement = document.querySelector('.h-captcha');
                if (captchaElement) {
                    analysis.captchaState = {
                        visible: captchaElement.offsetParent !== null,
                        sitekey: captchaElement.getAttribute('data-sitekey'),
                        size: captchaElement.getAttribute('data-size')
                    };
                }

                // Check for response textarea
                const responseTextarea = document.querySelector('[name="h-captcha-response"]');
                if (responseTextarea) {
                    analysis.captchaState.hasResponse = !!responseTextarea.value;
                    analysis.captchaState.responseLength = responseTextarea.value.length;
                }

                // Look for specific error patterns
                const bodyText = document.body.textContent.toLowerCase();
                if (bodyText.includes('erro na consulta')) {
                    analysis.errorPatterns = analysis.errorPatterns || [];
                    analysis.errorPatterns.push('ERRO_NA_CONSULTA');
                }
                if (bodyText.includes('cnpj não encontrado') || bodyText.includes('não foi possível')) {
                    analysis.errorPatterns = analysis.errorPatterns || [];
                    analysis.errorPatterns.push('CNPJ_NAO_ENCONTRADO');
                }
                if (bodyText.includes('captcha inválido') || bodyText.includes('captcha incorreto')) {
                    analysis.errorPatterns = analysis.errorPatterns || [];
                    analysis.errorPatterns.push('CAPTCHA_INVALIDO');
                }

                return analysis;
            });

            // Create comprehensive debug info
            const debugInfo = `
<!--
=== DEBUG INFO ===
Timestamp: ${timestamp}
CNPJ: ${cnpj}
URL: ${url}
Title: ${title}
Error Message: ${errorMessage}
Page Analysis: ${JSON.stringify(pageAnalysis, null, 2)}
===================
-->
${content}`;

            // Save HTML file
            const htmlFilename = `debug/error-${cnpj}-${timestamp}.html`;
            await fs.mkdir('debug', { recursive: true });
            await fs.writeFile(htmlFilename, debugInfo, 'utf-8');

            // Capture screenshot
            let screenshotFilename = null;
            try {
                screenshotFilename = `debug/screenshot-${cnpj}-${timestamp}.png`;
                await page.screenshot({
                    path: screenshotFilename,
                    fullPage: true,
                    type: 'png'
                });
                console.log(`CNPJService: 📸 Screenshot saved: ${screenshotFilename}`);
            } catch (screenshotError) {
                cnpjLogger.warn({ err: screenshotError }, 'Failed to capture screenshot');
                console.log(`CNPJService: ⚠️ Could not capture screenshot: ${screenshotError.message}`);
            }

            cnpjLogger.info({
                htmlFile: htmlFilename,
                screenshotFile: screenshotFilename,
                url,
                title,
                cnpj,
                errorMessage,
                pageAnalysis: {
                    errorsCount: pageAnalysis.errors.length,
                    visibleErrors: pageAnalysis.errors.filter(e => e.visible).length,
                    errorPatterns: pageAnalysis.errorPatterns || [],
                    pageType: pageAnalysis.pageContent.hasTable ? 'result' :
                             pageAnalysis.pageContent.hasForm ? 'form' : 'unknown',
                    captchaState: pageAnalysis.captchaState
                }
            }, 'Error page and screenshot saved for debugging');

            console.log(`CNPJService: 🔍 Debug files saved:`);
            console.log(`  📄 HTML: ${htmlFilename}`);
            if (screenshotFilename) {
                console.log(`  📸 Screenshot: ${screenshotFilename}`);
            }

        } catch (saveError) {
            cnpjLogger.error({ err: saveError }, 'Failed to save error page');
        }
    }

    /**
     * Legacy hCaptcha method - now using resilient SolveCaptchaService
     * This method is kept for compatibility but redirects to the new service
     */
    async solveHCaptcha(_apiKey, siteKey, pageUrl) {
        infoLog('CNPJService: Redirecting to resilient captcha service...');
        const result = await resilientCaptchaService.solveHCaptcha(siteKey, pageUrl);
        return result.token; // Return only token for backward compatibility
    }

    /**
     * Get captcha service health report
     */
    getCaptchaServiceHealth() {
        return resilientCaptchaService.getHealthReport();
    }

    /**
     * Validate captcha API key
     */
    async validateCaptchaApiKey() {
        try {
            return await resilientCaptchaService.validateApiKey();
        } catch (error) {
            cnpjLogger.error({ err: error }, 'Error validating captcha API key');
            return false;
        }
    }

    /**
     * Get captcha service balance
     */
    async getCaptchaBalance() {
        try {
            return await resilientCaptchaService.getBalance();
        } catch (error) {
            cnpjLogger.error({ err: error }, 'Error getting captcha balance');
            throw error;
        }
    }

    /**
     * Format the complete consultation result with extracted data
     */
    formatCompleteResult(dadosExtraidos, cnpj) {
        return {
            success: true,
            cnpj: cnpj,
            consultedAt: new Date().toISOString(),
            source: 'Receita Federal do Brasil',
            
            // Company identification
            identificacao: {
                cnpj: dadosExtraidos.cnpj || cnpj,
                tipo: dadosExtraidos.tipo || 'N/A',
                dataAbertura: dadosExtraidos.dataAbertura || 'N/A',
                nomeEmpresarial: dadosExtraidos.nomeEmpresarial || 'N/A',
                nomeFantasia: dadosExtraidos.nomeFantasia || 'N/A',
                porte: dadosExtraidos.porte || 'N/A',
                naturezaJuridica: dadosExtraidos.naturezaJuridica || 'N/A'
            },
            
            // Economic activities
            atividades: {
                principal: dadosExtraidos.atividadePrincipal || 'N/A',
                secundarias: dadosExtraidos.atividadesSecundarias || []
            },
            
            // Address information
            endereco: dadosExtraidos.endereco || {
                logradouro: 'N/A',
                numero: 'N/A',
                complemento: 'N/A',
                cep: 'N/A',
                bairro: 'N/A',
                municipio: 'N/A',
                uf: 'N/A'
            },
            
            // Contact information
            contato: dadosExtraidos.contato || {
                email: 'N/A',
                telefone: 'N/A'
            },
            
            // Registration status
            situacao: {
                cadastral: dadosExtraidos.situacaoCadastral || {
                    situacao: 'N/A',
                    data: 'N/A',
                    motivo: 'N/A'
                },
                especial: dadosExtraidos.situacaoEspecial || {
                    situacao: 'N/A',
                    data: 'N/A'
                }
            },
            
            // Additional information
            informacoesAdicionais: {
                enteFederativo: dadosExtraidos.enteFederativo || 'N/A',
                dataEmissao: dadosExtraidos.dataEmissao || null
            },
            
            // Metadata
            metadata: {
                extractionMethod: 'automated_browser_with_html_parsing',
                captchaSolved: true,
                dataQuality: 'high',
                extractedAt: new Date().toISOString(),
                version: '1.0.0'
            }
        };
    }



    /**
     * Clear cache for a specific CNPJ
     * @param {string} cnpj - CNPJ to clear from cache
     */
    clearCNPJCache(cnpj) {
        const cacheKey = cnpj.replace(/[^\d]/g, '');
        const deleted = cnpjCache.delete(cacheKey);
        if (deleted) {
            console.log(`CNPJService: Cache cleared for CNPJ ${cnpj}`);
        }
        return deleted;
    }

    /**
     * Get cache statistics
     */
    getCacheStats() {
        return cnpjCache.getStats();
    }

    /**
     * Clean expired cache entries
     */
    cleanExpiredCache() {
        const cleanedCount = cnpjCache.cleanup();
        if (cleanedCount > 0) {
            console.log(`CNPJService: Cleaned ${cleanedCount} expired cache entries`);
        }
        return cleanedCount;
    }

    /**
     * Intelligent adaptive delay utility with page condition awareness
     * @param {number|object} timeOrOptions - Time in ms or options object
     * @param {object} options - Additional options for adaptive behavior
     * @returns {Promise} Promise that resolves after the calculated delay
     */
    delay(timeOrOptions, options = {}) {
        let baseTime, minTime, maxTime, adaptive = false, pageCondition = null;

        // Handle different parameter formats
        if (typeof timeOrOptions === 'number') {
            baseTime = timeOrOptions;
            minTime = options.min || baseTime * 0.7; // More aggressive min
            maxTime = options.max || baseTime * 1.1; // Less aggressive max
            adaptive = options.adaptive || false;
            pageCondition = options.pageCondition;
        } else if (typeof timeOrOptions === 'object') {
            const opts = timeOrOptions;
            baseTime = opts.base || opts.time || 500; // Reduced default from 1000ms
            minTime = opts.min || baseTime * 0.7;
            maxTime = opts.max || baseTime * 1.1;
            adaptive = opts.adaptive || false;
            pageCondition = opts.pageCondition;
        } else {
            baseTime = 500; // Reduced default from 1000ms
            minTime = 350;
            maxTime = 550;
        }

        // ADAPTIVE DELAYS based on page conditions
        if (pageCondition) {
            switch (pageCondition) {
                case 'page_loading':
                    // Shorter delay for page loading checks
                    baseTime = Math.min(baseTime, 300);
                    break;
                case 'captcha_loading':
                    // Medium delay for captcha loading
                    baseTime = Math.min(baseTime, 800);
                    break;
                case 'form_submission':
                    // Longer delay for form submission
                    baseTime = Math.max(baseTime, 1000);
                    break;
                case 'error_recovery':
                    // Longer delay for error recovery
                    baseTime = Math.max(baseTime, 2000);
                    break;
                case 'extraction_ready':
                    // Very short delay for extraction
                    baseTime = Math.min(baseTime, 200);
                    break;
                default:
                    // Keep original baseTime
                    break;
            }
        }

        // Adaptive timing based on system performance
        if (adaptive && this.lastOperationTime) {
            const performanceFactor = Math.min(this.lastOperationTime / 1000, 1.5); // Reduced max factor
            baseTime = Math.floor(baseTime * performanceFactor);
        }

        // Natural randomization between min and max
        const finalDelay = Math.floor(minTime + Math.random() * (maxTime - minTime));

        return new Promise(resolve => setTimeout(resolve, finalDelay));
    }

    /**
     * Smart delay that adapts based on page readiness
     * @param {Object} page - Puppeteer page object
     * @param {string} condition - Condition to wait for
     * @param {number} maxWait - Maximum wait time
     * @returns {Promise} Promise that resolves when condition is met or timeout
     */
    async smartDelay(page, condition = 'ready', maxWait = 5000) {
        const startTime = Date.now();

        switch (condition) {
            case 'dom_ready':
                try {
                    await page.waitForFunction(() => document.readyState === 'complete', { timeout: maxWait });
                    const actualWait = Date.now() - startTime;
                    console.log(`CNPJService: DOM ready in ${actualWait}ms`);
                    return actualWait;
                } catch {
                    return maxWait;
                }

            case 'captcha_ready':
                try {
                    await page.waitForSelector('.h-captcha', { timeout: maxWait });
                    // Additional check for captcha iframe
                    await page.waitForFunction(() => {
                        const captcha = document.querySelector('.h-captcha');
                        return captcha && captcha.querySelector('iframe');
                    }, { timeout: 2000 });
                    const actualWait = Date.now() - startTime;
                    console.log(`CNPJService: Captcha ready in ${actualWait}ms`);
                    return actualWait;
                } catch {
                    return maxWait;
                }

            case 'form_ready':
                try {
                    await page.waitForSelector('button.btn-primary:not([disabled])', { timeout: maxWait });
                    const actualWait = Date.now() - startTime;
                    console.log(`CNPJService: Form ready in ${actualWait}ms`);
                    return actualWait;
                } catch {
                    return maxWait;
                }

            default:
                // Fallback to simple delay
                await this.delay(Math.min(maxWait, 500), { pageCondition: 'page_loading' });
                return Math.min(maxWait, 500);
        }
    }

    /**
     * Start data extraction with parallel processing (PERFORMANCE OPTIMIZATION)
     * @param {string} content - HTML content
     * @param {string} currentUrl - Current page URL
     * @param {string} cnpj - CNPJ being processed
     * @returns {Promise<Object>} Extracted data
     */
    async startDataExtraction(content, currentUrl, cnpj) {
        console.log(`CNPJService: Starting parallel data extraction for CNPJ ${cnpj}`);
        console.log(`CNPJService: Current URL: ${currentUrl}`);

        // PARALLELIZATION: Multiple extraction strategies simultaneously
        const extractionPromises = [];

        if (currentUrl.includes('Cnpjreva_Comprovante.asp')) {
            console.log('CNPJService: ✅ On result page, extracting data...');
            extractionPromises.push(
                extractorService.extrairDadosCNPJ('resultado-consulta.html')
                    .then(data => ({ source: 'direct', data }))
                    .catch(err => ({ source: 'direct', error: err.message }))
            );
        } else if (currentUrl.includes('valida_recaptcha.asp')) {
            console.log('CNPJService: 🔄 Still on validation page, trying multiple strategies...');

            // Strategy 1: Try extracting from current content
            extractionPromises.push(
                extractorService.extrairDadosCNPJ('resultado-consulta.html')
                    .then(data => ({ source: 'validation_page', data }))
                    .catch(err => ({ source: 'validation_page', error: err.message }))
            );
        } else {
            console.log('CNPJService: Unexpected page, trying extraction anyway...');
            extractionPromises.push(
                extractorService.extrairDadosCNPJ('resultado-consulta.html')
                    .then(data => ({ source: 'fallback', data }))
                    .catch(err => ({ source: 'fallback', error: err.message }))
            );
        }

        // Wait for all extraction attempts
        const results = await Promise.allSettled(extractionPromises);

        // Find first successful extraction
        for (const result of results) {
            if (result.status === 'fulfilled' && result.value.data) {
                console.log(`CNPJService: Data extraction successful via ${result.value.source} for CNPJ ${cnpj}`);
                return result.value.data;
            }
        }

        // If no successful extraction, log all errors
        console.log('CNPJService: All extraction strategies failed:');
        results.forEach((result, index) => {
            if (result.status === 'fulfilled' && result.value.error) {
                console.log(`  Strategy ${index + 1}: ${result.value.error}`);
            } else if (result.status === 'rejected') {
                console.log(`  Strategy ${index + 1}: ${result.reason}`);
            }
        });

        return null;
    }

    /**
     * Start background preloading for performance optimization
     * @returns {Promise<void>}
     */
    async startBackgroundPreloading() {
        try {
            // Preload browser pages for future use
            const resources = [
                {
                    type: 'browser_page',
                    browserPool: this.browserPool,
                    options: { preload: true }
                }
            ];

            await preloadService.preloadResources(resources);
            console.log('CNPJService: Background preloading completed');

        } catch (error) {
            console.log('CNPJService: Background preloading error:', error.message);
        }
    }

    /**
     * Get comprehensive service statistics including preload stats
     * @returns {Object} Service statistics
     */
    getServiceStats() {
        return {
            cache: cnpjCache.getStats(),
            preload: preloadService.getStats(),
            extractor: extractorService.getMemoryStats(),
            captcha: SolveCaptchaService.getHealthReport()
        };
    }

    /**
     * Clear cache for performance management
     */
    clearCache() {
        cnpjCache.clear();
        preloadService.clearAll();
        console.log('CNPJService: All caches cleared');
    }



    /**
     * Cleanup browser pool for maintenance
     */
    async cleanupBrowserPool() {
        await browserPool.cleanup();
        console.log('CNPJService: Browser pool cleaned up');
    }

    /**
     * Get browser pool statistics
     */
    getBrowserPoolStats() {
        return {
            poolSize: browserPool.pool.length,
            activeCount: browserPool.activeCount,
            maxSize: browserPool.maxSize
        };
    }

    /**
     * Analyze "Esclarecimentos adicionais" error to determine if it's recoverable
     * @param {Object} page - Puppeteer page
     * @param {string} cnpj - CNPJ being consulted
     * @returns {Promise<boolean>} True if error appears recoverable
     */
    async analyzeEsclarecimentosError(page, cnpj) {
        try {
            const errorAnalysis = await page.evaluate(() => {
                const errorMsg = document.querySelector('#msgTxtErroCaptcha, .alert-danger');
                const captchaFrame = document.querySelector('.h-captcha iframe');
                const captchaResponse = document.querySelector('textarea[name="h-captcha-response"]');

                const errorText = errorMsg ? errorMsg.textContent.toLowerCase() : '';

                // Check for specific CNPJ validation errors that are not recoverable
                const cnpjValidationErrors = [
                    'cnpj inválido',
                    'cnpj não encontrado',
                    'número de inscrição inválido',
                    'dados não localizados',
                    'situação cadastral inexistente'
                ];

                const isDefinitiveCnpjError = cnpjValidationErrors.some(pattern =>
                    errorText.includes(pattern)
                );

                return {
                    errorText: errorText,
                    hasCaptchaFrame: !!captchaFrame,
                    hasCaptchaResponse: !!(captchaResponse && captchaResponse.value),
                    isDefinitiveCnpjError: isDefinitiveCnpjError,
                    pageUrl: window.location.href,
                    pageTitle: document.title
                };
            });

            console.log('CNPJService: Error analysis for CNPJ', cnpj, ':', errorAnalysis);

            // If it's a definitive CNPJ validation error, don't retry
            if (errorAnalysis.isDefinitiveCnpjError) {
                console.log('CNPJService: ❌ Definitive CNPJ validation error detected - not recoverable');
                return false;
            }

            // If there's a captcha frame but no response, it might be a captcha issue
            if (errorAnalysis.hasCaptchaFrame && !errorAnalysis.hasCaptchaResponse) {
                console.log('CNPJService: 🔐 Captcha present but no response - likely captcha issue, recoverable');
                return true;
            }

            // Generic "Esclarecimentos adicionais" without specific CNPJ error - try once more
            console.log('CNPJService: ⚠️ Generic esclarecimentos error - attempting recovery');
            return true;

        } catch (error) {
            console.log('CNPJService: ⚠️ Error analyzing esclarecimentos error:', error.message);
            // If we can't analyze, assume it's recoverable and try once more
            return true;
        }
    }
}

module.exports = new CNPJService();