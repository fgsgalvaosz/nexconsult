# 📝 Sistema de Logs com Pino - CNPJ API

## 🎯 Visão Geral

O sistema de logs da CNPJ API utiliza **Pino** - o logger mais rápido para Node.js, com **pino-pretty** para formatação legível em desenvolvimento.

## 🚀 Vantagens do Pino

### **Performance**
- ✅ **5-10x mais rápido** que Winston
- ✅ **Baixo overhead** - não bloqueia o event loop
- ✅ **JSON nativo** - parsing mais eficiente
- ✅ **Streaming** - logs assíncronos

### **Funcionalidades**
- ✅ **Logs estruturados** em JSON
- ✅ **Child loggers** para correlação
- ✅ **Serializers** automáticos
- ✅ **Transports** flexíveis
- ✅ **Pretty printing** para desenvolvimento

## 🏗️ Arquitetura de Logs

### **Níveis de Log**
```javascript
{
  fatal: 60,    // 💥 Erros fatais
  error: 50,    // ❌ Erros
  warn: 40,     // ⚠️  Avisos
  info: 30,     // 📝 Informações
  debug: 20,    // 🐛 Debug
  trace: 10     // 🔍 Trace
}
```

### **Loggers Especializados**
- **`logger`** - Logger principal
- **`cnpjLogger`** - Operações de CNPJ
- **`extractorLogger`** - Extração de dados
- **`apiLogger`** - Requisições da API
- **`performanceLogger`** - Métricas de performance
- **`systemLogger`** - Sistema e lifecycle

## 📁 Estrutura de Arquivos

```
logs/
├── combined-YYYY-MM-DD.log    # Todos os logs (debug+)
├── app-YYYY-MM-DD.log         # Logs da aplicação (info+)
├── error-YYYY-MM-DD.log       # Apenas erros (error+)
└── ...                        # Arquivos por data
```

## 🔧 Configuração

### **Desenvolvimento**
```javascript
// Console com pino-pretty
{
  target: 'pino-pretty',
  options: {
    colorize: true,
    translateTime: 'HH:MM:ss',
    messageFormat: '[{service}] {msg}',
    levelFirst: true
  }
}
```

### **Produção**
```javascript
// Arquivos JSON estruturados
{
  target: 'pino/file',
  options: {
    destination: './logs/combined-2025-08-22.log'
  }
}
```

## 📊 Formato de Logs

### **Estrutura JSON**
```json
{
  "level": 30,
  "time": 1755879111302,
  "service": "cnpj-service",
  "correlationId": "exec_1755879111302_037fge",
  "cnpj": "48.123.272/0001-05",
  "executionId": "exec_1755879111302_037fge",
  "msg": "Starting complete consultation"
}
```

### **Console Pretty (Desenvolvimento)**
```
INFO [16:11:51]: [cnpj-service] Starting complete consultation
    service: "cnpj-service"
    correlationId: "exec_1755879111302_037fge"
    cnpj: "48.123.272/0001-05"
    executionId: "exec_1755879111302_037fge"
```

## 🔗 Correlação de Logs

### **Request ID**
Cada requisição HTTP recebe um ID único:
```javascript
req.requestId = Math.random().toString(36).substring(7);
```

### **Execution ID**
Cada consulta CNPJ recebe um ID de execução:
```javascript
const executionId = `exec_${startTime}_${Math.random().toString(36).substring(7)}`;
const correlatedLogger = createCorrelatedLogger(cnpjLogger, executionId);
```

### **Child Loggers**
```javascript
const correlatedLogger = logger.child({ 
  correlationId: executionId,
  service: 'cnpj-service'
});
```

## 🛠️ Comandos Úteis

### **Visualizar Logs em Tempo Real**
```bash
# Todos os logs com formatação
npm run logs

# Apenas erros
npm run logs:error

# Logs da aplicação
npm run logs:app

# Raw (sem formatação)
tail -f logs/combined-2025-08-22.log
```

### **Filtrar Logs**
```bash
# Por serviço
grep '"service":"cnpj-service"' logs/combined-2025-08-22.log | pino-pretty

# Por nível
grep '"level":50' logs/combined-2025-08-22.log | pino-pretty

# Por correlação
grep '"correlationId":"exec_123"' logs/combined-2025-08-22.log | pino-pretty
```

## 📡 API de Logs

### **Endpoint para Logs Recentes**
```http
GET /api/cnpj/logs/recent?level=info&limit=100
```

**Resposta:**
```json
{
  "success": true,
  "logs": [
    {
      "level": 30,
      "time": 1755879111302,
      "service": "cnpj-service",
      "msg": "Starting complete consultation"
    }
  ],
  "total": 50
}
```

## 🔍 Monitoramento

### **Métricas Automáticas**
```javascript
performanceLogger.debug({
  memory: {
    rss: 156.23,
    heapTotal: 89.45,
    heapUsed: 67.12
  },
  cpu: { user: 123456, system: 78910 },
  uptime: 3600,
  pid: 12345
}, 'System Metrics');
```

### **Performance Tracking**
```javascript
performanceLogger.info({
  cnpj: "48.123.272/0001-05",
  executionTime: 45230,
  success: true,
  cached: false,
  dataFields: 25
}, 'CNPJ consultation completed');
```

## 🚀 Benefícios Alcançados

### **Performance**
- ✅ **10x mais rápido** que Winston
- ✅ **Menor uso de CPU** durante logging
- ✅ **Não bloqueia** o event loop
- ✅ **Streaming assíncrono**

### **Desenvolvimento**
- ✅ **Logs coloridos** e legíveis
- ✅ **Correlação completa** de requisições
- ✅ **Debug eficiente** com context
- ✅ **Pretty printing** automático

### **Produção**
- ✅ **JSON estruturado** para parsing
- ✅ **Baixo overhead** de performance
- ✅ **Logs correlacionados** para troubleshooting
- ✅ **Métricas automáticas** de sistema

### **Troubleshooting**
- ✅ **Rastreamento completo** por correlationId
- ✅ **Context rico** em cada log
- ✅ **Filtros eficientes** por serviço/nível
- ✅ **Timeline clara** de execução

## 🔧 Configuração Avançada

### **Variáveis de Ambiente**
```bash
LOG_LEVEL=debug          # Nível mínimo de log
NODE_ENV=production      # Ambiente (afeta transports)
```

### **Customização**
```javascript
// Logger com context específico
const userLogger = logger.child({ 
  userId: '12345',
  sessionId: 'abc123'
});

// Log com dados estruturados
userLogger.info({
  action: 'cnpj_consultation',
  cnpj: '48.123.272/0001-05',
  duration: 1500
}, 'User performed CNPJ consultation');
```

O sistema Pino oferece logging de alta performance com correlação completa e debugging eficiente!
