const swaggerJsdoc = require('swagger-jsdoc');
const swaggerUi = require('swagger-ui-express');

const options = {
  definition: {
    openapi: '3.0.0',
    info: {
      title: 'CNPJ Consultation API',
      version: '1.0.0',
      description: 'API para consulta automática de CNPJ na Receita Federal do Brasil com extração completa de dados',
      contact: {
        name: 'CNPJ API Support',
        email: 'support@cnpjapi.com',
        url: 'https://github.com/seu-usuario/cnpj-api'
      },
      license: {
        name: 'MIT',
        url: 'https://opensource.org/licenses/MIT'
      }
    },
    servers: [
      {
        url: 'http://localhost:3000',
        description: 'Servidor de desenvolvimento'
      },
      {
        url: 'http://127.0.0.1:3000',
        description: 'Servidor de desenvolvimento (127.0.0.1)'
      },
      {
        url: 'https://api.cnpj.com',
        description: 'Servidor de produção'
      }
    ],
    components: {
      schemas: {
        CNPJRequest: {
          type: 'object',
          required: ['cnpj'],
          properties: {
            cnpj: {
              type: 'string',
              description: 'Número do CNPJ (com ou sem formatação)',
              example: '38.139.407/0001-77',
              pattern: '^\\d{2}\\.?\\d{3}\\.?\\d{3}\\/?\\d{4}\\-?\\d{2}$'
            },
            apiKey: {
              type: 'string',
              description: 'Chave de API opcional para resolução automática de captcha',
              example: 'bd238cb2bace2dd234e32a8df23486f1'
            }
          }
        },
        CNPJResponse: {
          type: 'object',
          properties: {
            success: {
              type: 'boolean',
              description: 'Indica se a consulta foi bem-sucedida',
              example: true
            },
            cnpj: {
              type: 'string',
              description: 'CNPJ consultado',
              example: '38.139.407/0001-77'
            },
            consultedAt: {
              type: 'string',
              format: 'date-time',
              description: 'Data e hora da consulta',
              example: '2025-08-22T14:00:00.000Z'
            },
            source: {
              type: 'string',
              description: 'Fonte dos dados',
              example: 'Receita Federal do Brasil'
            },
            identificacao: {
              type: 'object',
              properties: {
                cnpj: { type: 'string', example: '38.139.407/0001-77' },
                tipo: { type: 'string', example: 'MATRIZ' },
                dataAbertura: { type: 'string', example: '18/08/2020' },
                nomeEmpresarial: { type: 'string', example: 'FERRAZ AUTO CENTER LTDA' },
                nomeFantasia: { type: 'string', example: 'FERRAZ AUTO CENTER' },
                porte: { type: 'string', example: 'ME' },
                naturezaJuridica: { type: 'string', example: '206-2 - Sociedade Empresária Limitada' }
              }
            },
            atividades: {
              type: 'object',
              properties: {
                principal: { 
                  type: 'string', 
                  example: '45.30-7-05 - Comércio a varejo de pneumáticos e câmaras-de-ar' 
                },
                secundarias: {
                  type: 'array',
                  items: { type: 'string' },
                  example: [
                    '29.50-6-00 - Recondicionamento e recuperação de motores para veículos automotores',
                    '45.20-0-01 - Serviços de manutenção e reparação mecânica de veículos automotores'
                  ]
                }
              }
            },
            endereco: {
              type: 'object',
              properties: {
                logradouro: { type: 'string', example: 'R GUANABARA' },
                numero: { type: 'string', example: '377' },
                complemento: { type: 'string', example: '' },
                cep: { type: 'string', example: '65.913-447' },
                bairro: { type: 'string', example: 'ENTRONCAMENTO' },
                municipio: { type: 'string', example: 'IMPERATRIZ' },
                uf: { type: 'string', example: 'MA' }
              }
            },
            contato: {
              type: 'object',
              properties: {
                email: { type: 'string', example: '' },
                telefone: { type: 'string', example: '(99) 8160-6486' }
              }
            },
            situacao: {
              type: 'object',
              properties: {
                cadastral: {
                  type: 'object',
                  properties: {
                    situacao: { type: 'string', example: 'ATIVA' },
                    data: { type: 'string', example: '18/08/2020' },
                    motivo: { type: 'string', example: '' }
                  }
                },
                especial: {
                  type: 'object',
                  properties: {
                    situacao: { type: 'string', example: '' },
                    data: { type: 'string', example: '' }
                  }
                }
              }
            },
            informacoesAdicionais: {
              type: 'object',
              properties: {
                enteFederativo: { type: 'string', example: '' },
                dataEmissao: {
                  type: 'object',
                  properties: {
                    data: { type: 'string', example: '22/08/2025' },
                    hora: { type: 'string', example: '10:34:29' }
                  }
                }
              }
            },
            metadata: {
              type: 'object',
              properties: {
                extractionMethod: { type: 'string', example: 'automated_browser_with_html_parsing' },
                captchaSolved: { type: 'boolean', example: true },
                dataQuality: { type: 'string', example: 'high' },
                extractedAt: { type: 'string', format: 'date-time' },
                version: { type: 'string', example: '1.0.0' }
              }
            }
          }
        },
        Error: {
          type: 'object',
          properties: {
            error: {
              type: 'string',
              description: 'Tipo do erro',
              example: 'CNPJ inválido'
            },
            message: {
              type: 'string',
              description: 'Mensagem detalhada do erro',
              example: 'O CNPJ deve conter 14 dígitos'
            },
            timestamp: {
              type: 'string',
              format: 'date-time',
              description: 'Data e hora do erro'
            },
            path: {
              type: 'string',
              description: 'Caminho da requisição que gerou o erro',
              example: '/api/cnpj/consultar'
            },
            method: {
              type: 'string',
              description: 'Método HTTP da requisição',
              example: 'POST'
            }
          }
        },
        HealthCheck: {
          type: 'object',
          properties: {
            status: { type: 'string', example: 'healthy' },
            timestamp: { type: 'string', format: 'date-time' },
            uptime: { type: 'number', example: 3600.5 },
            environment: { type: 'string', example: 'development' },
            version: { type: 'string', example: '1.0.0' },
            memory: {
              type: 'object',
              properties: {
                used: { type: 'number', example: 45.67 },
                total: { type: 'number', example: 128.0 },
                external: { type: 'number', example: 12.34 }
              }
            },
            system: {
              type: 'object',
              properties: {
                platform: { type: 'string', example: 'win32' },
                arch: { type: 'string', example: 'x64' },
                nodeVersion: { type: 'string', example: 'v18.17.0' },
                pid: { type: 'number', example: 12345 }
              }
            }
          }
        }
      }
    }
  },
  apis: ['./src/routes/*.js', './src/controllers/*.js']
};

const specs = swaggerJsdoc(options);

module.exports = {
  specs,
  swaggerUi
};