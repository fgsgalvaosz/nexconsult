// Arquivo de configuração para a automação de consulta CNPJ

module.exports = {
    // Chave de API para resolver hCaptcha automaticamente
    // Substitua 'SUA_API_KEY_AQUI' pela sua chave de API real do solvecaptcha
    SOLVE_CAPTCHA_API_KEY: 'bd238cb2bace2dd234e32a8df23486f1',
    
    // CNPJ padrão para consulta
    DEFAULT_CNPJ: '38139407000177',
    
    // URL da página de consulta
    CONSULTA_URL: 'https://solucoes.receita.fazenda.gov.br/servicos/cnpjreva/cnpjreva_solicitacao.asp',

    // Configurações de resiliência
    CAPTCHA_MAX_RETRIES: 40,
    CAPTCHA_BASE_DELAY: 2000,
    CAPTCHA_MAX_BACKOFF: 10000
};