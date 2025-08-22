/**
 * Validation utilities
 */

/**
 * Validate CNPJ format and check digit
 * @param {string} cnpj - CNPJ to validate
 * @returns {Object} Validation result
 */
function validateCNPJ(cnpj) {
    if (!cnpj) {
        return {
            valid: false,
            message: 'CNPJ é obrigatório'
        };
    }

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
        formatted: cleanCNPJ.replace(/^(\d{2})(\d{3})(\d{3})(\d{4})(\d{2})$/, '$1.$2.$3/$4-$5'),
        clean: cleanCNPJ
    };
}

/**
 * Validate API key format
 * @param {string} apiKey - API key to validate
 * @returns {Object} Validation result
 */
function validateApiKey(apiKey) {
    if (!apiKey) {
        return {
            valid: false,
            message: 'API key é obrigatória'
        };
    }

    if (typeof apiKey !== 'string') {
        return {
            valid: false,
            message: 'API key deve ser uma string'
        };
    }

    if (apiKey.length < 10) {
        return {
            valid: false,
            message: 'API key deve ter pelo menos 10 caracteres'
        };
    }

    return {
        valid: true,
        message: 'API key válida'
    };
}

/**
 * Sanitize text input
 * @param {string} text - Text to sanitize
 * @returns {string} Sanitized text
 */
function sanitizeText(text) {
    if (!text || typeof text !== 'string') {
        return '';
    }
    
    return text
        .trim()
        .replace(/\s+/g, ' ')
        .replace(/[<>]/g, '')
        .substring(0, 1000); // Limit length
}

module.exports = {
    validateCNPJ,
    validateApiKey,
    sanitizeText
};