const axios = require('axios');
const FormData = require('form-data');
const { cnpjLogger } = require('../utils/logger');

/**
 * Serviço especializado para integração com SolveCaptcha API
 * Implementa padrões de resiliência: Circuit Breaker, Retry com Backoff, Fallbacks
 */
class SolveCaptchaService {
    constructor(apiKey) {
        this.apiKey = apiKey;
        this.baseUrl = 'https://api.solvecaptcha.com';
        
        // Configurações otimizadas com polling adaptativo
        this.config = {
            hcaptcha: {
                averageTime: 34000, // 34 segundos conforme documentação
                initialTimeout: 15000, // 15 segundos timeout inicial (otimizado)
                pollingInterval: {
                    initial: 3000, // 3 segundos iniciais (mais agressivo)
                    increment: 1000, // Incremento por tentativa
                    max: 8000 // Máximo 8 segundos
                },
                maxPollingAttempts: 25, // Aumentado para compensar polling adaptativo
                cost: 1.9, // $1.9 por 1000 captchas
                adaptivePolling: true // Habilitar polling adaptativo
            }
        };
        
        // Circuit Breaker
        this.circuitBreaker = {
            state: 'CLOSED', // CLOSED, OPEN, HALF_OPEN
            failureCount: 0,
            failureThreshold: 5,
            resetTimeout: 60000, // 1 minuto
            lastFailureTime: null
        };
        
        // Métricas avançadas
        this.metrics = {
            totalAttempts: 0,
            successfulSolves: 0,
            failedSolves: 0,
            averageResponseTime: 0,
            averageSolveTime: 0,
            fastestSolve: Infinity,
            slowestSolve: 0,
            errorsByType: new Map(),
            lastSuccessTime: null,
            pollingEfficiency: {
                totalPolls: 0,
                successfulPolls: 0,
                averagePollingTime: 0
            }
        };
        
        // Cache de soluções recentes (para fallback)
        this.solutionCache = new Map();
        this.cacheTimeout = 300000; // 5 minutos
    }

    /**
     * Resolve hCaptcha com resiliência completa
     * @param {string} siteKey - Site key do hCaptcha
     * @param {string} pageUrl - URL da página onde está o captcha
     * @param {Object} options - Opções adicionais
     * @returns {Promise<Object>} Resultado com token e user agent
     */
    async solveHCaptcha(siteKey, pageUrl, options = {}) {
        const startTime = Date.now();
        const operationId = `hcaptcha_${Date.now()}_${Math.random().toString(36).substring(7)}`;
        
        try {
            // Verificar circuit breaker
            this.checkCircuitBreaker();
            
            cnpjLogger.info({
                operationId,
                siteKey: siteKey.substring(0, 20) + '...',
                pageUrl,
                circuitState: this.circuitBreaker.state
            }, 'Iniciando resolução de hCaptcha');

            // Tentar resolver com retry automático
            const result = await this.solveWithRetry(siteKey, pageUrl, options, operationId);
            
            // Registrar sucesso
            this.recordSuccess(Date.now() - startTime);
            
            cnpjLogger.info({
                operationId,
                responseTime: Date.now() - startTime,
                hasUserAgent: !!result.useragent
            }, 'hCaptcha resolvido com sucesso');
            
            return result;
            
        } catch (error) {
            this.recordFailure(Date.now() - startTime, error);
            
            cnpjLogger.error({
                operationId,
                error: error.message,
                responseTime: Date.now() - startTime,
                circuitState: this.circuitBreaker.state
            }, 'Falha na resolução de hCaptcha');
            
            throw error;
        }
    }

    /**
     * Resolve hCaptcha com retry automático e backoff adaptativo
     */
    async solveWithRetry(siteKey, pageUrl, options, operationId) {
        let lastError;
        const maxRetries = 3; // Limite conservador para evitar rate limiting
        
        for (let attempt = 1; attempt <= maxRetries; attempt++) {
            try {
                return await this.executeSingleSolve(siteKey, pageUrl, options, operationId, attempt);
            } catch (error) {
                lastError = error;
                const errorType = this.classifyError(error);
                
                cnpjLogger.warn({
                    operationId,
                    attempt,
                    errorType: errorType.type,
                    shouldRetry: errorType.shouldRetry,
                    error: error.message
                }, 'Tentativa de resolução falhou');
                
                // Não tentar novamente se for erro permanente
                if (!errorType.shouldRetry || attempt === maxRetries) {
                    break;
                }
                
                // Calcular delay baseado no tipo de erro
                const delay = this.calculateRetryDelay(attempt, errorType.type);
                cnpjLogger.info({ operationId, delay, attempt }, 'Aguardando antes da próxima tentativa');
                await this.delay(delay);
            }
        }
        
        throw lastError;
    }

    /**
     * Executa uma única tentativa de resolução
     */
    async executeSingleSolve(siteKey, pageUrl, options, operationId, attempt) {
        // Passo 1: Submeter captcha para resolução
        const captchaId = await this.submitCaptcha(siteKey, pageUrl, options, operationId);
        
        cnpjLogger.info({
            operationId,
            captchaId,
            attempt
        }, 'Captcha submetido, aguardando resolução');
        
        // Passo 2: Aguardar timeout inicial (conforme documentação)
        await this.delay(this.config.hcaptcha.initialTimeout);
        
        // Passo 3: Polling para obter resultado
        return await this.pollForResult(captchaId, operationId);
    }

    /**
     * Submete captcha para a API
     */
    async submitCaptcha(siteKey, pageUrl, options, operationId) {
        const formData = new FormData();
        formData.append('key', this.apiKey);
        formData.append('method', 'hcaptcha');
        formData.append('sitekey', siteKey);
        formData.append('pageurl', pageUrl);
        formData.append('json', '1'); // Obrigatório para hCaptcha
        
        // Adicionar parâmetros opcionais
        if (options.invisible) {
            formData.append('invisible', '1');
        }
        if (options.domain) {
            formData.append('domain', options.domain);
        }
        if (options.data) {
            formData.append('data', options.data);
        }

        try {
            const response = await axios.post(`${this.baseUrl}/in.php`, formData, {
                headers: {
                    ...formData.getHeaders(),
                    'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
                },
                timeout: 30000
            });

            const result = response.data;
            
            if (result.status === 0) {
                const errorMsg = result.error_text || result.error || 'Erro desconhecido';
                throw new Error(`Erro na submissão: ${errorMsg}`);
            }
            
            if (result.status !== 1 || !result.request) {
                throw new Error(`Resposta inesperada da API: ${JSON.stringify(result)}`);
            }
            
            return result.request; // ID do captcha
            
        } catch (error) {
            if (error.response) {
                throw new Error(`HTTP ${error.response.status}: ${error.response.statusText}`);
            }
            throw error;
        }
    }

    /**
     * Faz polling adaptativo para obter o resultado do captcha
     */
    async pollForResult(captchaId, operationId) {
        const maxAttempts = this.config.hcaptcha.maxPollingAttempts;
        const pollingStartTime = Date.now();
        let totalPollingTime = 0;

        for (let attempt = 1; attempt <= maxAttempts; attempt++) {
            const pollStartTime = Date.now();

            try {
                const response = await axios.get(`${this.baseUrl}/res.php`, {
                    params: {
                        key: this.apiKey,
                        action: 'get',
                        id: captchaId,
                        json: 1 // Obrigatório para hCaptcha
                    },
                    timeout: 15000,
                    headers: {
                        'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
                    }
                });

                const result = response.data;
                this.metrics.pollingEfficiency.totalPolls++;

                // Status 0 = erro ou não pronto
                if (result.status === 0) {
                    const errorText = result.error_text || result.request || 'Erro desconhecido';

                    if (errorText === 'CAPCHA_NOT_READY') {
                        cnpjLogger.debug({
                            operationId,
                            captchaId,
                            attempt,
                            maxAttempts,
                            elapsedTime: Date.now() - pollingStartTime
                        }, 'Captcha ainda não está pronto');

                        if (attempt < maxAttempts) {
                            // Polling adaptativo - aumenta intervalo gradualmente
                            const delay = this.calculateAdaptivePollingDelay(attempt);
                            totalPollingTime += delay;
                            await this.delay(delay);
                            continue;
                        } else {
                            throw new Error('Timeout: Captcha não foi resolvido no tempo esperado');
                        }
                    }
                    
                    // Erro específico de rate limiting
                    if (errorText.includes('1005')) {
                        throw new Error('RATE_LIMIT: Muitas requisições - API bloqueada por 5 minutos');
                    }
                    
                    throw new Error(`Erro na verificação: ${errorText}`);
                }
                
                // Status 1 = sucesso
                if (result.status === 1 && result.request) {
                    const totalSolveTime = Date.now() - pollingStartTime + this.config.hcaptcha.initialTimeout;
                    this.metrics.pollingEfficiency.successfulPolls++;
                    this.updatePollingMetrics(totalPollingTime, totalSolveTime);

                    cnpjLogger.info({
                        operationId,
                        captchaId,
                        attempt,
                        totalTime: totalSolveTime,
                        pollingTime: totalPollingTime,
                        hasUserAgent: !!result.useragent
                    }, 'hCaptcha resolvido com sucesso');

                    // Cache da solução para possível reuso
                    this.cacheSolution(captchaId, result);

                    return {
                        token: result.request,
                        useragent: result.useragent,
                        respKey: result.respKey,
                        captchaId: captchaId
                    };
                }
                
                throw new Error(`Resposta inesperada: ${JSON.stringify(result)}`);
                
            } catch (error) {
                if (attempt === maxAttempts) {
                    throw error;
                }
                
                // Para erros de rede, aguardar antes de tentar novamente
                if (error.code === 'ETIMEDOUT' || error.code === 'ECONNRESET') {
                    await this.delay(this.config.hcaptcha.pollingInterval);
                } else {
                    throw error; // Outros erros não devem ser retentados no polling
                }
            }
        }
        
        throw new Error('Máximo de tentativas de polling excedido');
    }

    /**
     * Classifica erros para determinar estratégia de retry
     */
    classifyError(error) {
        const message = error.message.toLowerCase();

        // Erros permanentes - não tentar novamente
        if (message.includes('wrong_user_key') ||
            message.includes('key_does_not_exist') ||
            message.includes('zero_balance')) {
            return { type: 'PERMANENT', shouldRetry: false };
        }

        // Rate limiting - retry com backoff agressivo
        if (message.includes('1005') || message.includes('rate_limit')) {
            return { type: 'RATE_LIMIT', shouldRetry: true };
        }

        // Erros de servidor - retry com backoff exponencial
        if (message.includes('http 5') || message.includes('econnrefused')) {
            return { type: 'SERVER_ERROR', shouldRetry: true };
        }

        // Erros de rede - retry com backoff linear
        if (message.includes('timeout') || message.includes('enotfound')) {
            return { type: 'NETWORK_ERROR', shouldRetry: true };
        }

        return { type: 'UNKNOWN', shouldRetry: true };
    }

    /**
     * Calcula delay adaptativo para polling baseado na tentativa
     */
    calculateAdaptivePollingDelay(attempt) {
        if (!this.config.hcaptcha.adaptivePolling) {
            return this.config.hcaptcha.pollingInterval.initial;
        }

        const { initial, increment, max } = this.config.hcaptcha.pollingInterval;
        const adaptiveDelay = initial + (increment * (attempt - 1));

        // Adicionar jitter para distribuir requests
        const jitter = adaptiveDelay * 0.1 * Math.random();

        return Math.min(adaptiveDelay + jitter, max);
    }

    /**
     * Calcula delay para retry baseado no tipo de erro
     */
    calculateRetryDelay(attempt, errorType) {
        const baseDelay = 5000; // 5 segundos
        const maxDelay = 30000; // 30 segundos

        let delay;
        switch (errorType) {
            case 'RATE_LIMIT':
                delay = baseDelay * Math.pow(2, attempt) * 2; // Backoff agressivo
                break;
            case 'SERVER_ERROR':
                delay = baseDelay * Math.pow(1.5, attempt); // Backoff exponencial
                break;
            case 'NETWORK_ERROR':
                delay = baseDelay * attempt; // Backoff linear
                break;
            default:
                delay = baseDelay;
        }

        // Adicionar jitter para evitar thundering herd
        const jitter = delay * 0.1 * Math.random();
        return Math.min(delay + jitter, maxDelay);
    }

    /**
     * Atualiza métricas de polling
     */
    updatePollingMetrics(pollingTime, totalSolveTime) {
        // Atualizar métricas de tempo de resolução
        this.metrics.averageSolveTime = this.metrics.averageSolveTime === 0
            ? totalSolveTime
            : (this.metrics.averageSolveTime + totalSolveTime) / 2;

        this.metrics.fastestSolve = Math.min(this.metrics.fastestSolve, totalSolveTime);
        this.metrics.slowestSolve = Math.max(this.metrics.slowestSolve, totalSolveTime);

        // Atualizar métricas de polling
        this.metrics.pollingEfficiency.averagePollingTime =
            this.metrics.pollingEfficiency.averagePollingTime === 0
                ? pollingTime
                : (this.metrics.pollingEfficiency.averagePollingTime + pollingTime) / 2;
    }

    /**
     * Verifica e gerencia circuit breaker
     */
    checkCircuitBreaker() {
        if (this.circuitBreaker.state === 'OPEN') {
            const timeSinceLastFailure = Date.now() - this.circuitBreaker.lastFailureTime;

            if (timeSinceLastFailure >= this.circuitBreaker.resetTimeout) {
                this.circuitBreaker.state = 'HALF_OPEN';
                cnpjLogger.info('Circuit breaker mudou para HALF_OPEN - testando serviço');
            } else {
                const remainingTime = Math.ceil((this.circuitBreaker.resetTimeout - timeSinceLastFailure) / 1000);
                throw new Error(`Circuit breaker OPEN - serviço indisponível por mais ${remainingTime}s`);
            }
        }
    }

    /**
     * Registra sucesso e atualiza circuit breaker
     */
    recordSuccess(responseTime) {
        this.metrics.totalAttempts++;
        this.metrics.successfulSolves++;
        this.metrics.lastSuccessTime = Date.now();
        this.updateAverageResponseTime(responseTime);

        // Circuit breaker
        if (this.circuitBreaker.state === 'HALF_OPEN') {
            this.circuitBreaker.state = 'CLOSED';
            this.circuitBreaker.failureCount = 0;
            cnpjLogger.info('Circuit breaker FECHADO - serviço restaurado');
        } else if (this.circuitBreaker.state === 'CLOSED') {
            this.circuitBreaker.failureCount = Math.max(0, this.circuitBreaker.failureCount - 1);
        }
    }

    /**
     * Registra falha e atualiza circuit breaker
     */
    recordFailure(responseTime, error) {
        this.metrics.totalAttempts++;
        this.metrics.failedSolves++;
        this.updateAverageResponseTime(responseTime);

        const errorType = this.classifyError(error).type;
        const count = this.metrics.errorsByType.get(errorType) || 0;
        this.metrics.errorsByType.set(errorType, count + 1);

        // Circuit breaker
        this.circuitBreaker.failureCount++;
        this.circuitBreaker.lastFailureTime = Date.now();

        if (this.circuitBreaker.failureCount >= this.circuitBreaker.failureThreshold) {
            this.circuitBreaker.state = 'OPEN';
            cnpjLogger.error({
                failureCount: this.circuitBreaker.failureCount,
                threshold: this.circuitBreaker.failureThreshold
            }, 'Circuit breaker ABERTO - serviço temporariamente desabilitado');
        }
    }

    /**
     * Atualiza tempo médio de resposta
     */
    updateAverageResponseTime(responseTime) {
        if (this.metrics.averageResponseTime === 0) {
            this.metrics.averageResponseTime = responseTime;
        } else {
            // Média móvel simples
            this.metrics.averageResponseTime = (this.metrics.averageResponseTime * 0.8) + (responseTime * 0.2);
        }
    }

    /**
     * Cache de soluções para fallback
     */
    cacheSolution(captchaId, solution) {
        this.solutionCache.set(captchaId, {
            solution,
            timestamp: Date.now()
        });

        // Limpar cache antigo
        this.cleanupCache();
    }

    /**
     * Limpa soluções antigas do cache
     */
    cleanupCache() {
        const now = Date.now();
        for (const [id, cached] of this.solutionCache.entries()) {
            if (now - cached.timestamp > this.cacheTimeout) {
                this.solutionCache.delete(id);
            }
        }
    }

    /**
     * Utilitário para delay
     */
    delay(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    /**
     * Obtém relatório avançado de saúde do serviço
     */
    getHealthReport() {
        const successRate = this.metrics.totalAttempts > 0
            ? (this.metrics.successfulSolves / this.metrics.totalAttempts) * 100
            : 0;

        const pollingEfficiency = this.metrics.pollingEfficiency.totalPolls > 0
            ? (this.metrics.pollingEfficiency.successfulPolls / this.metrics.pollingEfficiency.totalPolls) * 100
            : 0;

        return {
            circuitBreakerState: this.circuitBreaker.state,
            totalAttempts: this.metrics.totalAttempts,
            successfulSolves: this.metrics.successfulSolves,
            failedSolves: this.metrics.failedSolves,
            successRate: `${successRate.toFixed(2)}%`,
            averageResponseTime: `${Math.round(this.metrics.averageResponseTime)}ms`,
            averageSolveTime: `${Math.round(this.metrics.averageSolveTime)}ms`,
            fastestSolve: this.metrics.fastestSolve === Infinity ? 'N/A' : `${Math.round(this.metrics.fastestSolve)}ms`,
            slowestSolve: `${Math.round(this.metrics.slowestSolve)}ms`,
            pollingEfficiency: {
                totalPolls: this.metrics.pollingEfficiency.totalPolls,
                successfulPolls: this.metrics.pollingEfficiency.successfulPolls,
                efficiency: `${pollingEfficiency.toFixed(2)}%`,
                averagePollingTime: `${Math.round(this.metrics.pollingEfficiency.averagePollingTime)}ms`
            },
            lastSuccessTime: this.metrics.lastSuccessTime,
            errorsByType: Object.fromEntries(this.metrics.errorsByType),
            cacheSize: this.solutionCache.size,
            estimatedCostPer1000: this.config.hcaptcha.cost,
            adaptivePolling: this.config.hcaptcha.adaptivePolling
        };
    }

    /**
     * Reporta resultado para a API (boa prática)
     */
    async reportResult(captchaId, isCorrect) {
        try {
            const action = isCorrect ? 'reportgood' : 'reportbad';
            await axios.get(`${this.baseUrl}/res.php`, {
                params: {
                    key: this.apiKey,
                    action: action,
                    id: captchaId
                },
                timeout: 10000
            });

            cnpjLogger.info({ captchaId, action }, 'Resultado reportado para SolveCaptcha');
        } catch (error) {
            cnpjLogger.warn({ captchaId, error: error.message }, 'Falha ao reportar resultado');
        }
    }

    /**
     * Obtém saldo da conta
     */
    async getBalance() {
        try {
            const response = await axios.get(`${this.baseUrl}/res.php`, {
                params: {
                    key: this.apiKey,
                    action: 'getbalance',
                    json: 1
                },
                timeout: 10000
            });

            const balance = parseFloat(response.data.request || response.data);
            cnpjLogger.info({ balance }, 'Saldo da conta SolveCaptcha obtido');
            return balance;
        } catch (error) {
            cnpjLogger.error({ error: error.message }, 'Erro ao obter saldo');
            throw error;
        }
    }

    /**
     * Valida se a API key está funcionando
     */
    async validateApiKey() {
        try {
            await this.getBalance();
            return true;
        } catch (error) {
            if (error.message.includes('WRONG_USER_KEY') ||
                error.message.includes('KEY_DOES_NOT_EXIST')) {
                return false;
            }
            throw error; // Outros erros podem ser temporários
        }
    }

    /**
     * Método de conveniência para integração com código existente
     * Mantém compatibilidade com a interface atual
     */
    async solveHCaptchaLegacy(apiKey, siteKey, pageUrl) {
        // Temporariamente usar a API key fornecida se diferente da configurada
        const originalApiKey = this.apiKey;
        if (apiKey && apiKey !== this.apiKey) {
            this.apiKey = apiKey;
        }

        try {
            const result = await this.solveHCaptcha(siteKey, pageUrl);
            return result.token; // Retorna apenas o token para compatibilidade
        } finally {
            this.apiKey = originalApiKey; // Restaurar API key original
        }
    }
}

module.exports = SolveCaptchaService;
