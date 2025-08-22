const puppeteer = require('puppeteer');
const extractorService = require('./extractorService');
const config = require('../../config');
const fs = require('fs').promises;
const request = require('request');

class CNPJService {
    /**
     * Perform complete CNPJ consultation with data extraction
     * @param {string} cnpj - CNPJ number to consult
     * @param {string} apiKey - Optional API key for captcha solving
     * @returns {Promise<Object>} Complete consultation result with extracted data
     */
    async consultarCNPJ(cnpj, apiKey = null) {
        let browser;
        try {
            // Use provided API key or default from config
            const chaveApi = apiKey || config.SOLVE_CAPTCHA_API_KEY;
            
            console.log(`CNPJService: Starting complete consultation for CNPJ ${cnpj}`);
            
            // Step 1: Perform the consultation with improved logic
            browser = await this.launchBrowser();
            const page = await browser.newPage();
            
            // Configure page
            await this.configurePage(page);
            
            // Navigate to consultation page
            await this.navigateToConsultationPage(page);
            
            // Fill CNPJ
            await this.fillCNPJ(page, cnpj);
            
            // Solve hCaptcha
            await this.solveCaptcha(page, chaveApi);
            
            // Submit form and wait for results
            const resultPage = await this.submitFormAndWaitForResults(page);
            
            // Step 2: Extract data from the result page
            let dadosExtraidos = null;
            if (resultPage) {
                // Save HTML content
                const content = await page.content();
                await fs.writeFile('resultado-consulta.html', content, 'utf-8');
                console.log('CNPJService: Result page saved to resultado-consulta.html');
                
                // Extract data
                dadosExtraidos = await extractorService.extrairDadosCNPJ('resultado-consulta.html');
                console.log(`CNPJService: Data extraction successful for CNPJ ${cnpj}`);
            }
            
            if (dadosExtraidos) {
                return this.formatCompleteResult(dadosExtraidos, cnpj);
            } else {
                console.log(`CNPJService: No data found for CNPJ ${cnpj}`);
                return null;
            }
            
        } catch (error) {
            console.error(`CNPJService: Error in complete consultation for CNPJ ${cnpj}:`, error);
            throw new Error(`Failed to consult CNPJ: ${error.message}`);
        } finally {
            if (browser) {
                console.log('CNPJService: Closing browser...');
                try {
                    await browser.close();
                } catch (closeError) {
                    console.error('CNPJService: Error closing browser:', closeError.message);
                }
            }
        }
    }

    /**
     * Launch browser with optimized settings
     */
    async launchBrowser() {
        console.log('CNPJService: Launching browser...');
        return await puppeteer.launch({
            headless: 'new', // Use new headless mode for better compatibility
            args: [
                '--no-sandbox',
                '--disable-setuid-sandbox',
                '--disable-dev-shm-usage',
                '--disable-accelerated-2d-canvas',
                '--no-first-run',
                '--no-zygote',
                '--single-process',
                '--disable-gpu',
                '--disable-blink-features=AutomationControlled',
                '--disable-web-security',
                '--disable-features=VizDisplayCompositor',
                '--disable-background-timer-throttling',
                '--disable-backgrounding-occluded-windows',
                '--disable-renderer-backgrounding',
                '--disable-field-trial-config',
                '--disable-ipc-flooding-protection'
            ],
            ignoreDefaultArgs: ['--disable-extensions'],
            timeout: 60000
        });
    }

    /**
     * Configure page to avoid detection
     */
    async configurePage(page) {
        console.log('CNPJService: Configuring page...');
        
        // Avoid automation detection
        await page.evaluateOnNewDocument(() => {
            Object.defineProperty(navigator, 'webdriver', {
                get: () => undefined,
            });
            
            // Remove automation indicators
            delete window.chrome;
            window.chrome = {
                runtime: {}
            };
        });
        
        // Set user agent
        await page.setUserAgent('Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36');
    }

    /**
     * Navigate to consultation page
     */
    async navigateToConsultationPage(page) {
        console.log('CNPJService: Navigating to consultation page...');
        await page.goto(config.CONSULTA_URL, {
            waitUntil: 'networkidle2',
            timeout: 60000
        });
        console.log('CNPJService: Page loaded successfully');
    }

    /**
     * Fill CNPJ field
     */
    async fillCNPJ(page, cnpj) {
        console.log('CNPJService: Filling CNPJ field...');
        await page.waitForSelector('#cnpj', { timeout: 10000 });
        await page.focus('#cnpj');
        await page.type('#cnpj', cnpj, { delay: 100 });
        console.log('CNPJService: CNPJ field filled successfully');
    }

    /**
     * Solve hCaptcha with improved detection
     */
    async solveCaptcha(page, apiKey) {
        console.log('CNPJService: Starting hCaptcha resolution...');
        
        if (apiKey) {
            try {
                // Wait for hCaptcha to load
                await page.waitForSelector('.h-captcha', { timeout: 15000 });
                await this.delay(3000); // Wait for complete loading
                
                console.log('CNPJService: hCaptcha element found, extracting site key...');
                
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
                    
                    // Solve hCaptcha
                    const captchaResponse = await this.solveHCaptcha(apiKey, siteKey, page.url());
                    
                    // Inject response
                    await page.evaluate((response) => {
                        // Try multiple methods to inject response
                        const textarea = document.querySelector('[name="h-captcha-response"]');
                        if (textarea) {
                            textarea.value = response;
                            console.log('hCaptcha response injected via name attribute');
                        }
                        
                        // Try by ID pattern
                        const textareaById = document.querySelector('textarea[id*="h-captcha-response"]');
                        if (textareaById) {
                            textareaById.value = response;
                            console.log('hCaptcha response injected via ID pattern');
                        }
                        
                        // Trigger events
                        if (window.hcaptcha) {
                            try {
                                window.hcaptcha.execute();
                                console.log('hCaptcha execute triggered');
                            } catch (e) {
                                console.log('Error executing hcaptcha:', e);
                            }
                        }
                    }, captchaResponse);
                    
                    console.log('CNPJService: hCaptcha solved and injected successfully');
                    await this.delay(2000); // Wait for processing
                } else {
                    console.log('CNPJService: Could not find hCaptcha site key, falling back to manual resolution');
                    await this.handleManualCaptcha(page);
                }
            } catch (error) {
                console.log(`CNPJService: Error solving hCaptcha automatically: ${error.message}`);
                console.log('CNPJService: Falling back to manual resolution...');
                await this.handleManualCaptcha(page);
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
        console.log('CNPJService: Submitting form...');
        
        try {
            // Wait for submit button
            await page.waitForSelector('button.btn-primary', { timeout: 10000 });
            
            // Set up navigation promise before clicking
            const navigationPromise = page.waitForNavigation({
                waitUntil: 'domcontentloaded',
                timeout: 120000 // 2 minutes timeout
            });
            
            // Click the button
            await page.click('button.btn-primary');
            console.log('CNPJService: Form submitted, waiting for navigation...');
            
            // Wait for navigation to complete
            await navigationPromise;
            console.log('CNPJService: Navigation completed');
            
            // Wait for page to stabilize
            await this.delay(5000);
            
            // Check current URL
            const currentUrl = page.url();
            console.log(`CNPJService: Current URL: ${currentUrl}`);
            
            if (currentUrl.includes('Cnpjreva_Comprovante.asp')) {
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
                
                // Check for error messages
                const hasErrors = await page.evaluate(() => {
                    const errorDiv = document.querySelector('#msgErroCaptcha');
                    if (errorDiv && errorDiv.style.display !== 'none') {
                        return { error: true, message: errorDiv.textContent.trim() };
                    }
                    
                    const msgErro = document.querySelector('#msgErro');
                    if (msgErro && !msgErro.classList.contains('collapse')) {
                        return { error: true, message: msgErro.textContent.trim() };
                    }
                    
                    return { error: false };
                });
                
                if (hasErrors.error) {
                    throw new Error(`Consultation error: ${hasErrors.message}`);
                }
                
                return false;
            } else {
                console.log('CNPJService: Unexpected page reached');
                return false;
            }
            
        } catch (error) {
            console.error(`CNPJService: Error during form submission: ${error.message}`);
            
            // Save error page if possible
            try {
                if (page && !page.isClosed()) {
                    const content = await page.content();
                    await fs.writeFile('pagina-erro.html', content, 'utf-8');
                    console.log('CNPJService: Error page saved');
                }
            } catch (saveError) {
                console.error('CNPJService: Could not save error page:', saveError.message);
            }
            
            throw error;
        }
    }

    /**
     * Solve hCaptcha using external API
     */
    async solveHCaptcha(apiKey, siteKey, pageUrl) {
        return new Promise((resolve, reject) => {
            console.log('CNPJService: Sending hCaptcha to solving service...');
            
            const options = {
                'method': 'POST',
                'url': 'https://api.solvecaptcha.com/in.php',
                'headers': {},
                formData: {
                    'key': apiKey,
                    'method': 'hcaptcha',
                    'sitekey': siteKey,
                    'pageurl': pageUrl
                }
            };
            
            request(options, (error, response) => {
                if (error) {
                    reject(new Error('Error sending captcha solve request: ' + error.message));
                    return;
                }
                
                try {
                    const result = response.body.trim();
                    console.log('CNPJService: Captcha service response:', result);
                    
                    if (result.startsWith('ERROR')) {
                        reject(new Error('Captcha service error: ' + result));
                        return;
                    }
                    
                    if (!result.startsWith('OK|')) {
                        reject(new Error('Unexpected captcha service response: ' + result));
                        return;
                    }
                    
                    const captchaId = result.split('|')[1];
                    console.log('CNPJService: Captcha ID obtained:', captchaId);
                    
                    // Check result
                    const checkResult = () => {
                        const getResultOptions = {
                            'method': 'GET',
                            'url': `https://api.solvecaptcha.com/res.php?key=${apiKey}&action=get&id=${captchaId}`
                        };
                        
                        request(getResultOptions, (error, response) => {
                            if (error) {
                                reject(new Error('Error checking captcha result: ' + error.message));
                                return;
                            }
                            
                            const resultText = response.body.trim();
                            console.log('CNPJService: Captcha check response:', resultText);
                            
                            if (resultText.startsWith('ERROR')) {
                                reject(new Error('Error checking captcha result: ' + resultText));
                                return;
                            }
                            
                            if (resultText === 'CAPCHA_NOT_READY') {
                                console.log('CNPJService: Captcha not ready, waiting...');
                                setTimeout(checkResult, 5000);
                                return;
                            }
                            
                            if (resultText.startsWith('OK|')) {
                                const captchaResponse = resultText.split('|')[1];
                                console.log('CNPJService: Captcha solved successfully!');
                                resolve(captchaResponse);
                            } else {
                                reject(new Error('Unexpected captcha check response: ' + resultText));
                            }
                        });
                    };
                    
                    // Start checking after 5 seconds
                    setTimeout(checkResult, 5000);
                } catch (parseError) {
                    reject(new Error('Error parsing captcha service response: ' + parseError.message));
                }
            });
        });
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
     * Delay utility function
     */
    delay(time) {
        return new Promise(resolve => setTimeout(resolve, time));
    }
}

module.exports = new CNPJService();