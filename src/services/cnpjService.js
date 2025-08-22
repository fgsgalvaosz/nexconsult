const puppeteer = require('puppeteer');
const extractorService = require('./extractorService');
const config = require('../config');
const fs = require('fs').promises;
const { cnpjLogger, performanceLogger, createCorrelatedLogger } = require('../utils/logger');

// Simple in-memory cache for CNPJ results
const cnpjCache = new Map();
const CACHE_DURATION = 30 * 60 * 1000; // 30 minutes
const axios = require('axios');
const FormData = require('form-data');

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
     * Validate CNPJ format and check digit
     */
    validateCNPJ(cnpj) {
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

        // Validate check digits
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
            return { valid: false, error: 'Dígito verificador inválido' };
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
            return { valid: false, error: 'Dígito verificador inválido' };
        }

        return { valid: true };
    }

    /**
     * Perform complete CNPJ consultation with data extraction
     * @param {string} cnpj - CNPJ number to consult
     * @param {string} apiKey - Optional API key for captcha solving
     * @returns {Promise<Object>} Complete consultation result with extracted data
     */
    async consultarCNPJ(cnpj, apiKey = null) {
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

            const executionId = `exec_${startTime}_${Math.random().toString(36).substring(7)}`;
            const correlatedLogger = createCorrelatedLogger(cnpjLogger, executionId);

            correlatedLogger.info({ cnpj, executionId }, 'Starting complete consultation');

            // Check cache first for performance optimization
            const cacheKey = cnpj.replace(/[^\d]/g, ''); // Clean CNPJ for cache key
            const cachedResult = cnpjCache.get(cacheKey);

            if (cachedResult && (Date.now() - cachedResult.timestamp) < CACHE_DURATION) {
                const cacheAge = Date.now() - cachedResult.timestamp;
                correlatedLogger.info({ cnpj, cacheAge, executionId }, 'Cache hit - returning cached result');
                return cachedResult.data;
            }

            // Step 1: Perform the consultation with improved logic
            browser = await browserPool.getBrowser();
            const page = await browser.newPage();

            // Set performance settings with URL logging
            await page.setRequestInterception(true);
            page.on('request', (req) => {
                const resourceType = req.resourceType();
                const url = req.url();

                // Log important requests
                if (resourceType === 'document' || url.includes('cnpj') || url.includes('consulta')) {
                    console.log(`CNPJService: 📡 Request: ${resourceType} -> ${url}`);
                }

                // Só bloquear imagens para melhor performance, mas manter CSS e JS
                if (['image', 'media'].includes(resourceType)) {
                    req.abort();
                } else {
                    req.continue();
                }
            });

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
            
            // Configure page
            await this.configurePage(page);

            // Navigate directly to consultation page with CNPJ pre-filled
            await this.navigateToConsultationPageWithCNPJ(page, cnpj);

            // Check if captcha is present before trying to solve it
            const hasCaptcha = await this.checkIfCaptchaIsPresent(page);
            console.log(`CNPJService: Captcha present: ${hasCaptcha}`);

            if (hasCaptcha) {
                // Solve hCaptcha only if present
                await this.solveCaptcha(page, chaveApi);
            } else {
                console.log('CNPJService: No captcha detected, proceeding directly to form submission');
            }

            // Submit form and wait for results
            await this.submitFormAndWaitForResults(page);

            // Step 2: Extract data from the result page
            let dadosExtraidos = null;

            // Always try to extract data, even if resultPage is false
            try {
                // Wait a bit more to ensure page is fully loaded (optimized)
                await this.delay(1000); // Reduced from 2000ms to 1000ms

                // Save HTML content for debugging in parallel
                const contentPromise = page.content();
                const urlPromise = page.url();

                const [content, currentUrl] = await Promise.all([contentPromise, urlPromise]);

                // Save file asynchronously without waiting
                fs.writeFile('resultado-consulta.html', content, 'utf-8').catch(err =>
                    console.log('CNPJService: Warning - could not save HTML file:', err.message)
                );
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

                        // Check if it's a captcha error that we can retry
                        const captchaErrors = pageErrors.filter(e =>
                            e.text.includes('Erro na Consulta') ||
                            e.text.includes('captcha') ||
                            e.text.includes('Esclarecimentos')
                        );

                        if (captchaErrors.length > 0) {
                            console.log('CNPJService: 🔄 Captcha error detected, attempting comprehensive retry...');

                            // Try a more comprehensive retry approach
                            try {
                                // Wait a bit longer for any pending operations
                                await this.delay(3000);

                                // Clear any existing captcha responses
                                await page.evaluate(() => {
                                    const textareas = document.querySelectorAll('textarea[name="h-captcha-response"], textarea[id*="h-captcha-response"]');
                                    textareas.forEach(ta => ta.value = '');
                                });

                                // Solve captcha again with fresh approach
                                console.log('CNPJService: 🔄 Re-solving captcha with fresh approach...');
                                await this.solveCaptcha(page, chaveApi);

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
                                        // Save error page with screenshot for CNPJ not found
                                        await this.saveErrorPage(page, cnpj, `CNPJ not found: ${cnpjNotFoundError.text}`);
                                        throw new Error(`CNPJ não encontrado na base da Receita Federal: ${cnpjNotFoundError.text}`);
                                    } else {
                                        // Save retry failure page with screenshot for other errors
                                        await this.saveErrorPage(page, cnpj, `Retry failed: ${retryErrors.map(e => e.text).join(', ')}`);
                                        throw new Error(`Captcha retry failed - still on form page. Errors: ${retryErrors.map(e => e.text).join(', ')}`);
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

                // Cache the result for future requests
                cnpjCache.set(cacheKey, {
                    data: result,
                    timestamp: Date.now()
                });

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
                console.log(`CNPJService: No data found for CNPJ ${cnpj}`);
                return null;
            }
            
        } catch (error) {
            console.error(`CNPJService: Error in complete consultation for CNPJ ${cnpj}:`, error);

            // Capture screenshot and HTML for any error
            try {
                if (page && !page.isClosed()) {
                    await this.captureErrorDebugFiles(page, cnpj, error, 'consultation-error');
                }
            } catch (debugError) {
                console.log('CNPJService: Could not capture debug files:', debugError.message);
            }

            // Check if it's a captcha-related error
            if (error.message.includes('captcha') || error.message.includes('Esclarecimentos') || error.message.includes('Navigation failed')) {
                console.log('CNPJService: 🔄 Captcha-related error detected');

                // Return a structured error response instead of throwing
                const totalTime = Date.now() - startTime;
                return {
                    success: false,
                    error: 'CAPTCHA_VALIDATION_FAILED',
                    message: 'A Receita Federal rejeitou a validação do captcha. Isso pode ser devido a mudanças no sistema ou alta demanda.',
                    details: {
                        originalError: error.message,
                        cnpj: cnpj,
                        executionTime: totalTime,
                        timestamp: new Date().toISOString(),
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
                        version: '2.0.0'
                    }
                };
            }

            throw new Error(`Failed to consult CNPJ: ${error.message}`);
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
     * Configure page to avoid detection with performance optimizations
     */
    async configurePage(page) {
        console.log('CNPJService: Configuring page...');

        // Set natural viewport similar to the recorded session
        await page.setViewport({ width: 1100, height: 639 });

        // Enable natural browser features
        await page.setJavaScriptEnabled(true);
        await page.setCacheEnabled(true); // Enable cache for natural behavior

        // Make browser appear more natural
        await page.evaluateOnNewDocument(() => {
            // Remove webdriver property
            Object.defineProperty(navigator, 'webdriver', {
                get: () => undefined,
            });

            // Make chrome object more natural
            if (!window.chrome) {
                window.chrome = {};
            }
            if (!window.chrome.runtime) {
                window.chrome.runtime = {};
            }

            // Natural plugins array
            Object.defineProperty(navigator, 'plugins', {
                get: () => [
                    { name: 'Chrome PDF Plugin' },
                    { name: 'Chrome PDF Viewer' },
                    { name: 'Native Client' }
                ],
            });

            // Natural languages
            Object.defineProperty(navigator, 'languages', {
                get: () => ['pt-BR', 'pt', 'en-US', 'en'],
            });

            // Natural permissions
            if (navigator.permissions && navigator.permissions.query) {
                const originalQuery = navigator.permissions.query;
                navigator.permissions.query = (parameters) => (
                    parameters.name === 'notifications' ?
                        Promise.resolve({ state: 'default' }) :
                        originalQuery(parameters)
                );
            }
        });

        // Natural user agent
        await page.setUserAgent('Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36');

        // Natural headers
        await page.setExtraHTTPHeaders({
            'Accept-Language': 'pt-BR,pt;q=0.9,en;q=0.8',
            'Accept': 'text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7'
        });
    }

    /**
     * Check if captcha is present on the page
     */
    async checkIfCaptchaIsPresent(page) {
        try {
            // Wait a bit for page to load completely
            await this.delay(1000);

            // Check for hCaptcha element
            const captchaElement = await page.$('.h-captcha');
            if (captchaElement) {
                console.log('CNPJService: hCaptcha element found');
                return true;
            }

            // Check for other captcha indicators
            const captchaScript = await page.$('script[src*="hcaptcha"]');
            if (captchaScript) {
                console.log('CNPJService: hCaptcha script found');
                return true;
            }

            // Check if form can be submitted directly
            const submitButton = await page.$('button.btn-primary');
            if (submitButton) {
                const isEnabled = await page.evaluate((btn) => {
                    return !btn.disabled && btn.offsetParent !== null;
                }, submitButton);

                if (isEnabled) {
                    console.log('CNPJService: Submit button is enabled, likely no captcha required');
                    return false;
                }
            }

            console.log('CNPJService: No captcha elements detected');
            return false;

        } catch (error) {
            console.log('CNPJService: Error checking for captcha, assuming captcha is present:', error.message);
            return true; // Default to assuming captcha is present for safety
        }
    }

    /**
     * Navigate to consultation page with optimized loading
     */
    async navigateToConsultationPage(page) {
        console.log('CNPJService: Navigating to consultation page...');

        // Set up parallel promises for faster loading
        const navigationPromise = page.goto(config.CONSULTA_URL, {
            waitUntil: 'domcontentloaded',
            timeout: 25000 // Further reduced timeout
        });

        // Wait for essential elements in parallel
        const selectorPromise = page.waitForSelector('#cnpj', { timeout: 25000 });

        // Execute both promises in parallel
        await Promise.all([navigationPromise, selectorPromise]);

        console.log('CNPJService: Page loaded successfully with essential elements ready');
    }

    /**
     * Navigate directly to consultation page with CNPJ pre-filled (OTIMIZADO!)
     */
    async navigateToConsultationPageWithCNPJ(page, cnpj) {
        console.log(`CNPJService: 🚀 Navigating directly to consultation page with CNPJ ${cnpj}...`);

        // Construir URL otimizada com CNPJ pré-preenchido
        const optimizedUrl = `${config.CONSULTA_URL}?cnpj=${cnpj}`;
        console.log(`CNPJService: ⚡ Using optimized URL: ${optimizedUrl}`);

        // Navegar diretamente para URL com CNPJ
        const navigationPromise = page.goto(optimizedUrl, {
            waitUntil: 'domcontentloaded',
            timeout: 25000
        });

        // Aguardar elementos essenciais (captcha é mais importante que campo CNPJ)
        const captchaPromise = page.waitForSelector('.h-captcha', { timeout: 25000 });

        // Executar em paralelo
        await Promise.all([navigationPromise, captchaPromise]);

        console.log('CNPJService: ✅ Optimized page loaded - CNPJ pre-filled, captcha ready!');

        // Verificar se CNPJ foi realmente preenchido
        const cnpjValue = await page.evaluate(() => {
            const cnpjField = document.querySelector('#cnpj');
            return cnpjField ? cnpjField.value : null;
        });

        console.log(`CNPJService: 📋 CNPJ field value: ${cnpjValue}`);

        // Se não foi preenchido automaticamente, preencher manualmente
        if (!cnpjValue || cnpjValue.replace(/\D/g, '') !== cnpj.replace(/\D/g, '')) {
            console.log('CNPJService: ⚠️ CNPJ not auto-filled, filling manually...');
            await this.fillCNPJ(page, cnpj);
        } else {
            console.log('CNPJService: ✅ CNPJ successfully pre-filled via URL!');
        }
    }

    /**
     * Fill CNPJ field with optimized input
     */
    async fillCNPJ(page, cnpj) {
        console.log('CNPJService: Filling CNPJ field...');

        // Clear any existing value and input directly
        await page.evaluate((cnpjValue) => {
            const input = document.querySelector('#cnpj');
            if (input) {
                input.value = '';
                input.value = cnpjValue;
                input.dispatchEvent(new Event('input', { bubbles: true }));
                input.dispatchEvent(new Event('change', { bubbles: true }));
            }
        }, cnpj);

        console.log('CNPJService: CNPJ field filled successfully');
    }

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
                await this.delay(1000); // Further reduced from 1500ms to 1000ms

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

                    // Solve hCaptcha
                    console.log('CNPJService: Calling solveHCaptcha method...');
                    const captchaResponse = await this.solveHCaptcha(apiKey, siteKey, page.url());
                    console.log(`CNPJService: Captcha response received: ${captchaResponse ? captchaResponse.substring(0, 50) + '...' : 'null'}`);
                    
                    // Inject response with improved method
                    const injectionResult = await page.evaluate((response) => {
                        console.log('Injecting hCaptcha response:', response.substring(0, 50) + '...');

                        // Method 1: Find all textareas with h-captcha-response
                        const textareas = document.querySelectorAll('textarea[name="h-captcha-response"], textarea[id*="h-captcha-response"]');
                        console.log('Found textareas:', textareas.length);

                        let injectedCount = 0;
                        textareas.forEach((textarea, index) => {
                            // Set value directly
                            textarea.value = response;

                            // Also set as property
                            textarea.setAttribute('value', response);

                            // Trigger multiple events
                            textarea.dispatchEvent(new Event('input', { bubbles: true }));
                            textarea.dispatchEvent(new Event('change', { bubbles: true }));
                            textarea.dispatchEvent(new Event('blur', { bubbles: true }));

                            console.log(`hCaptcha response injected in textarea ${index + 1}`);
                            console.log(`Textarea ${index + 1} value after injection:`, textarea.value.substring(0, 50) + '...');
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
                    }, captchaResponse);

                    console.log('CNPJService: Injection result:', JSON.stringify(injectionResult, null, 2));
                    
                    console.log('CNPJService: hCaptcha solved and injected successfully');
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
                } else {
                    cnpjLogger.error('Could not find hCaptcha site key on page');
                    throw new Error('hCaptcha site key not found - cannot proceed with API resolution');
                }
            } catch (error) {
                cnpjLogger.error({ err: error }, 'Failed to solve captcha via API');

                // Capture screenshot and HTML of captcha error since we have page access here
                try {
                    if (page && !page.isClosed()) {
                        await this.captureErrorDebugFiles(page, cnpj, error, 'captcha-error');
                    }
                } catch (captureError) {
                    cnpjLogger.warn({ err: captureError }, 'Failed to capture captcha error debug files');
                }

                throw new Error(`Captcha API resolution failed: ${error.message}`);
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

            // Wait a bit more to ensure everything is ready (optimized)
            console.log('CNPJService: Waiting 1 second before submission...');
            await this.delay(1000); // Increased back to 1000ms for better reliability

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
                            throw new Error(`CNPJ não encontrado na base da Receita Federal: ${hasErrors.message}`);

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
     * Solve hCaptcha using external API with Axios
     */
    async solveHCaptcha(apiKey, siteKey, pageUrl) {
        try {
            cnpjLogger.debug({
                apiKey: apiKey ? apiKey.substring(0, 8) + '...' : 'null',
                siteKey,
                pageUrl
            }, 'Sending hCaptcha to solving service');

            // Criar FormData para o request
            const formData = new FormData();
            formData.append('key', apiKey);
            formData.append('method', 'hcaptcha');
            formData.append('sitekey', siteKey);
            formData.append('pageurl', pageUrl);
            formData.append('json', '1');

            cnpjLogger.debug('Sending request to captcha solving service');

            // Enviar captcha para resolução
            const submitResponse = await axios.post('https://api.solvecaptcha.com/in.php', formData, {
                headers: {
                    ...formData.getHeaders(),
                    'Content-Type': 'multipart/form-data',
                    'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
                },
                timeout: 30000
            });

            const result = submitResponse.data;
            cnpjLogger.debug({ result }, 'Captcha service response');
            console.log('CNPJService: SolveCaptcha API response:', JSON.stringify(result, null, 2));

            if (result.status === 0) {
                const errorMsg = result.error_text || result.error || 'Unknown error';
                console.log(`CNPJService: SolveCaptcha API error: ${errorMsg}`);
                throw new Error(`Captcha service error: ${errorMsg}`);
            }

            if (result.status !== 1) {
                throw new Error(`Unexpected captcha service response: ${JSON.stringify(result)}`);
            }

            const captchaId = result.request;
            cnpjLogger.info({ captchaId }, 'Captcha ID obtained');

            // Aguardar e verificar resultado
            return await this.checkCaptchaResult(apiKey, captchaId);

        } catch (error) {
            cnpjLogger.error({ err: error }, 'Error solving hCaptcha');
            throw new Error(`Failed to solve hCaptcha: ${error.message}`);
        }
    }

    /**
     * Check captcha result using Axios
     */
    async checkCaptchaResult(apiKey, captchaId) {
        // Otimizado para hCaptcha baseado na documentação SolveCaptcha:
        // - Velocidade média: 31 segundos
        // - Máximo 20 requests por captcha para evitar bloqueio ERROR: 1005
        const maxAttempts = 15; // Margem de segurança (15 < 20)
        const baseDelay = 5000; // 5 segundos conforme documentação
        const initialDelay = 20000; // 20 segundos timeout inicial para hCaptcha

        // Timeout inicial específico para hCaptcha conforme documentação
        console.log(`CNPJService: Initial ${initialDelay/1000}s timeout for hCaptcha as per SolveCaptcha documentation...`);
        await this.delay(initialDelay);

        for (let attempt = 1; attempt <= maxAttempts; attempt++) {
            try {
                // Aguardar 5 segundos entre tentativas conforme documentação
                if (attempt > 1) {
                    await this.delay(baseDelay);
                }

                cnpjLogger.debug({
                    attempt,
                    maxAttempts,
                    captchaId,
                    elapsedTime: `${initialDelay/1000 + (attempt-1) * baseDelay/1000}s`,
                    avgHCaptchaTime: '31s'
                }, 'Checking hCaptcha result');

                const response = await axios.get('https://api.solvecaptcha.com/res.php', {
                    params: {
                        key: apiKey,
                        action: 'get',
                        id: captchaId,
                        json: 1  // Obrigatório para hCaptcha
                    },
                    timeout: 15000, // Aumentado para 15s
                    headers: {
                        'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
                    }
                });

                const result = response.data;
                cnpjLogger.debug({ result, attempt }, 'Captcha check response');

                // Para hCaptcha com json=1, sempre recebemos JSON
                if (typeof result === 'object' && result !== null) {
                    // Status 0 = erro ou não pronto
                    if (result.status === 0) {
                        const errorText = result.error_text || result.request || 'Unknown error';
                        if (errorText === 'CAPCHA_NOT_READY') {
                            cnpjLogger.debug({ attempt }, 'hCaptcha not ready, waiting...');
                            continue;
                        } else if (errorText.includes('1005')) {
                            cnpjLogger.error({
                                captchaId,
                                attempt,
                                error: errorText
                            }, 'SolveCaptcha ERROR 1005: Too many requests - API blocked for 5 minutes');
                            throw new Error(`SolveCaptcha blocked (ERROR 1005): Too many requests. Wait 5 minutes before retrying.`);
                        } else {
                            throw new Error(`hCaptcha check error: ${errorText}`);
                        }
                    }

                    // Status 1 = sucesso
                    if (result.status === 1) {
                        cnpjLogger.info({
                            captchaId,
                            attempt,
                            totalTime: `${initialDelay/1000 + attempt * baseDelay/1000}s`,
                            avgHCaptchaTime: '31s',
                            hasUserAgent: !!result.useragent,
                            hasRespKey: !!result.respKey
                        }, 'hCaptcha solved successfully!');

                        // Para hCaptcha, retornamos o token principal
                        return result.request;
                    }
                }

                // Fallback para formato string (compatibilidade)
                if (typeof result === 'string') {
                    if (result === 'CAPCHA_NOT_READY') {
                        cnpjLogger.debug({ attempt }, 'Captcha not ready, waiting...');
                        continue;
                    } else if (result.startsWith('OK|')) {
                        const captchaResponse = result.split('|')[1];
                        cnpjLogger.info({ captchaId, attempt }, 'Captcha solved successfully!');
                        return captchaResponse;
                    } else if (result.startsWith('ERROR')) {
                        throw new Error(`Captcha check error: ${result}`);
                    }
                }

                throw new Error(`Unexpected captcha check response: ${JSON.stringify(result)}`);

            } catch (error) {
                // Verificar se é erro 521 (servidor indisponível)
                const is521Error = error.response?.status === 521;
                const isNetworkError = error.code === 'ECONNREFUSED' || error.code === 'ENOTFOUND' || error.code === 'ETIMEDOUT';

                if (attempt === maxAttempts) {
                    const totalTime = initialDelay/1000 + maxAttempts * baseDelay/1000;
                    cnpjLogger.error({
                        err: error,
                        captchaId,
                        attempt,
                        totalTime: `${totalTime}s`,
                        avgHCaptchaTime: '31s',
                        note: 'hCaptcha timeout - may need to increase timeout or check API status'
                    }, 'Failed to check hCaptcha result after max attempts');
                    throw new Error(`hCaptcha timeout after ${totalTime}s (avg: 31s): ${error.message}`);
                }

                if (is521Error || isNetworkError) {
                    // Backoff exponencial para erros de servidor
                    const backoffDelay = Math.min(baseDelay * Math.pow(1.5, attempt - 1), 10000); // Max 10s
                    cnpjLogger.warn({
                        attempt,
                        err: error,
                        nextDelay: backoffDelay,
                        errorType: is521Error ? 'SERVER_DOWN_521' : 'NETWORK_ERROR'
                    }, 'Server/network error, using exponential backoff...');

                    await this.delay(backoffDelay);
                } else {
                    // Outros erros - delay normal
                    cnpjLogger.warn({ err: error, attempt }, 'Error checking captcha result, retrying...');
                    await this.delay(baseDelay);
                }
            }
        }

        // Final timeout error with enhanced logging
        const totalTime = initialDelay/1000 + maxAttempts * baseDelay/1000;
        cnpjLogger.error({
            captchaId,
            maxAttempts,
            totalTime: `${totalTime}s`,
            avgHCaptchaTime: '31s',
            note: 'hCaptcha timeout - may need to increase timeout or check API status'
        }, 'Captcha check timeout - max attempts reached');

        throw new Error(`hCaptcha timeout after ${totalTime}s (avg: 31s) - max attempts reached`);
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
     * Validate CNPJ format and check digit
     */
    validateCNPJ(cnpj) {
        // Remove non-numeric characters
        const cleanCNPJ = cnpj.replace(/\D/g, '');
        
        // Check length
        if (cleanCNPJ.length !== 14) {
            return {
                valid: false,
                message: 'CNPJ deve conter 14 dígitos'
            };
        }

        // Check if all digits are the same
        if (/^(\d)\1+$/.test(cleanCNPJ)) {
            return {
                valid: false,
                message: 'CNPJ inválido - todos os dígitos são iguais'
            };
        }

        // Validate check digits
        const digits = cleanCNPJ.split('').map(Number);
        
        // First check digit
        let sum = 0;
        let weight = 5;
        for (let i = 0; i < 12; i++) {
            sum += digits[i] * weight;
            weight = weight === 2 ? 9 : weight - 1;
        }
        const firstCheck = sum % 11 < 2 ? 0 : 11 - (sum % 11);
        
        if (firstCheck !== digits[12]) {
            return {
                valid: false,
                message: 'CNPJ inválido - primeiro dígito verificador incorreto'
            };
        }

        // Second check digit
        sum = 0;
        weight = 6;
        for (let i = 0; i < 13; i++) {
            sum += digits[i] * weight;
            weight = weight === 2 ? 9 : weight - 1;
        }
        const secondCheck = sum % 11 < 2 ? 0 : 11 - (sum % 11);
        
        if (secondCheck !== digits[13]) {
            return {
                valid: false,
                message: 'CNPJ inválido - segundo dígito verificador incorreto'
            };
        }

        return {
            valid: true,
            message: 'CNPJ válido',
            formatted: cleanCNPJ.replace(/^(\d{2})(\d{3})(\d{3})(\d{4})(\d{2})$/, '$1.$2.$3/$4-$5')
        };
    }

    /**
     * Natural delay utility function with randomization
     */
    delay(time) {
        // Add natural randomization (±10%)
        const randomFactor = 0.9 + Math.random() * 0.2; // 0.9 to 1.1
        const naturalDelay = Math.floor(time * randomFactor);
        return new Promise(resolve => setTimeout(resolve, naturalDelay));
    }

    /**
     * Human-like delay between actions
     */
    async naturalDelay(minMs = 500, maxMs = 1500) {
        const delay = minMs + Math.random() * (maxMs - minMs);
        return new Promise(resolve => setTimeout(resolve, Math.floor(delay)));
    }

    /**
     * Clear cache for performance management
     */
    clearCache() {
        cnpjCache.clear();
        console.log('CNPJService: Cache cleared');
    }

    /**
     * Get cache statistics
     */
    getCacheStats() {
        return {
            size: cnpjCache.size,
            keys: Array.from(cnpjCache.keys())
        };
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
}

module.exports = new CNPJService();