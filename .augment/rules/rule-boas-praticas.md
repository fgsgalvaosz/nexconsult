---
type: "manual"
---

# 📘 Guia Compacto — Menos Código

## 🎯 Objetivo
Reduzir **complexidade** e **duplicação**, mantendo a **funcionalidade**.

---

## 🧭 Princípios
- **KISS** – Keep It Simple, Stupid  
- **YAGNI** – You Aren’t Gonna Need It  
- **DRY** – Don’t Repeat Yourself  
- **SRP** – Single Responsibility Principle  

---

## ✅ Regras Obrigatórias
- Funções **≤ 30 linhas** (máx **50**).  
- **Sem duplicação**: repetição ≥ 2 vezes → **abstrair**.  
- Nomes **claros e explicativos**.  
- Funções públicas **não devem usar flags booleanas**.  
- Pull Request (PR) **≤ 300 linhas** (ideal: **<150**).  
- **Linter/formatter** integrado ao CI (merge bloqueado se falhar).  

---

## 💡 Boas Práticas
- Só abstrair **após repetição real**.  
- Usar **objeto/DTO** se houver **>4 parâmetros**.  
- Preferir **composição > herança**.  
- Manter **commits atômicos**.  

---

## 📝 Checklist de PR
- [ ] Título e descrição claros  
- [ ] Link para a issue associada  
- [ ] Linter/formatter executados  
- [ ] Justificar se a PR > 300 linhas  

---

## 🔄 Integração Contínua (CI)
- Executar:
  - Linter  
  - Análise de duplicação  
- **Merge bloqueado** em caso de falha  

---

## 📊 Métricas-Chave
- **Linhas por arquivo**  
- **Duplicação de código**  
- **Funções com complexidade ciclomática > 10**  
- **Tempo médio de review**  

---

## ⚡ Exemplo Rápido
```js
const TAXA = 0.08;

function aplicarTaxa(valor, taxa = TAXA) {
  return valor + valor * taxa;
}
