const config = require('../config');

/**
 * NavigationService - Specialized service for browser navigation and page configuration
 * Handles all navigation logic, form filling, and page setup operations
 */
class NavigationService {
    constructor() {
        this.isDevelopment = process.env.NODE_ENV !== 'production';
        this.debugLog = this.isDevelopment ? console.log : () => {};
        this.infoLog = console.log; // Always log navigation info
    }

    /**
     * Configure page to avoid detection with performance optimizations
     */
    async configurePage(page) {
        this.debugLog('NavigationService: Configuring page...');

        // Set natural viewport similar to the recorded session
        await page.setViewport({ width: 1100, height: 639 });

        // Enable natural browser features
        await page.setJavaScriptEnabled(true);
        await page.setCacheEnabled(true); // Enable cache for natural behavior

        // Stealth configurations to avoid detection
        await page.evaluateOnNewDocument(() => {
            // Remove webdriver property
            Object.defineProperty(navigator, 'webdriver', {
                get: () => undefined,
            });

            // Mock plugins
            Object.defineProperty(navigator, 'plugins', {
                get: () => [1, 2, 3, 4, 5],
            });

            // Mock languages
            Object.defineProperty(navigator, 'languages', {
                get: () => ['pt-BR', 'pt', 'en'],
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

        this.debugLog('NavigationService: Page configuration completed');
    }

    /**
     * Navigate directly to consultation page with CNPJ pre-filled (OPTIMIZED!)
     */
    async navigateToConsultationPageWithCNPJ(page, cnpj) {
        this.infoLog(`NavigationService: 🚀 Navigating directly to consultation page with CNPJ ${cnpj}...`);

        // Construir URL otimizada com CNPJ pré-preenchido
        const optimizedUrl = `${config.CONSULTA_URL}?cnpj=${cnpj}`;
        this.debugLog(`NavigationService: ⚡ Using optimized URL: ${optimizedUrl}`);

        // Navegar diretamente para URL com CNPJ
        const navigationPromise = page.goto(optimizedUrl, {
            waitUntil: 'domcontentloaded',
            timeout: 25000
        });

        // Aguardar elementos essenciais (captcha é mais importante que campo CNPJ)
        const captchaPromise = page.waitForSelector('.h-captcha', { timeout: 25000 });

        // Executar em paralelo
        await Promise.all([navigationPromise, captchaPromise]);

        this.infoLog('NavigationService: ✅ Optimized page loaded - CNPJ pre-filled, captcha ready!');

        // Verificar se CNPJ foi realmente preenchido
        const cnpjValue = await page.evaluate(() => {
            const cnpjField = document.querySelector('#cnpj');
            return cnpjField ? cnpjField.value : null;
        });

        this.debugLog(`NavigationService: 📋 CNPJ field value: ${cnpjValue}`);

        // Se não foi preenchido automaticamente, preencher manualmente
        if (!cnpjValue || cnpjValue.replace(/\D/g, '') !== cnpj.replace(/\D/g, '')) {
            this.debugLog('NavigationService: ⚠️ CNPJ not auto-filled, filling manually...');
            await this.fillCNPJ(page, cnpj);
        } else {
            this.infoLog('NavigationService: ✅ CNPJ successfully pre-filled via URL!');
        }
    }

    /**
     * Fill CNPJ field with optimized input
     */
    async fillCNPJ(page, cnpj) {
        this.debugLog('NavigationService: Filling CNPJ field...');

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

        this.debugLog('NavigationService: CNPJ field filled successfully');
    }

    /**
     * Setup performance-optimized request interception
     */
    async setupRequestInterception(page) {
        await page.setRequestInterception(true);
        
        page.on('request', (req) => {
            const resourceType = req.resourceType();
            const url = req.url();

            // Log important requests (only in development)
            if (resourceType === 'document' || url.includes('cnpj') || url.includes('consulta')) {
                this.debugLog(`NavigationService: 📡 Request: ${resourceType} -> ${url}`);
            }

            // Só bloquear imagens para melhor performance, mas manter CSS e JS
            if (['image', 'media'].includes(resourceType)) {
                req.abort();
            } else {
                req.continue();
            }
        });
    }

    /**
     * Check if captcha is present on the page
     */
    async checkIfCaptchaIsPresent(page) {
        try {
            await page.waitForSelector('.h-captcha', { timeout: 3000 });
            return true;
        } catch (error) {
            return false;
        }
    }

    /**
     * Wait for page to be ready for form submission
     */
    async waitForPageReady(page, timeout = 10000) {
        try {
            // Wait for submit button to be available
            await page.waitForSelector('button.btn-primary', { timeout });
            
            // Additional check for page stability
            await page.evaluate(() => {
                return new Promise((resolve) => {
                    if (document.readyState === 'complete') {
                        resolve();
                    } else {
                        window.addEventListener('load', resolve);
                    }
                });
            });
            
            return true;
        } catch (error) {
            this.debugLog(`NavigationService: Page ready check failed: ${error.message}`);
            return false;
        }
    }

    /**
     * Get current page information for debugging
     */
    async getPageInfo(page) {
        try {
            const url = page.url();
            const title = await page.title();
            return { url, title };
        } catch (error) {
            return { url: 'unknown', title: 'unknown', error: error.message };
        }
    }
}

module.exports = new NavigationService();
