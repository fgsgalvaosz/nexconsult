"""
CNPJ Consultor V2 - Sistema Ultra Robusto e Performático
Consulta CNPJ na Receita Federal com máxima confiabilidade
"""

import asyncio
import json
import logging
import re
import time
from dataclasses import dataclass, asdict
from typing import Dict, List, Optional, Union
from pathlib import Path

import requests
from playwright.async_api import async_playwright, Browser, Page, Playwright


@dataclass
class CNPJData:
    """Estrutura de dados do CNPJ"""
    cnpj: Dict[str, str]
    empresa: Dict[str, Union[str, Dict]]
    atividades: Dict[str, Union[Dict, List]]
    endereco: Dict[str, str]
    contato: Dict[str, str]
    situacao: Dict[str, str]
    comprovante: Dict[str, str]
    metadados: Dict[str, Union[str, bool]]


class SolveCaptchaClient:
    """Cliente otimizado para API SolveCaptcha"""
    
    def __init__(self, api_key: str, timeout: int = 300):
        self.api_key = api_key
        self.timeout = timeout
        self.base_url = "https://api.solvecaptcha.com"
        self.session = requests.Session()
        self.session.headers.update({'User-Agent': 'CNPJConsultor/2.0'})
    
    async def solve_hcaptcha(self, sitekey: str, pageurl: str) -> Optional[str]:
        """Resolve hCaptcha com retry automático"""
        max_retries = 3
        
        for attempt in range(max_retries):
            try:
                # Submete captcha
                captcha_id = await self._submit_captcha(sitekey, pageurl)
                if not captcha_id:
                    continue
                
                # Aguarda resolução
                token = await self._wait_solution(captcha_id)
                if token:
                    return token
                    
            except Exception as e:
                logging.warning(f"Tentativa {attempt + 1} falhou: {e}")
                if attempt < max_retries - 1:
                    await asyncio.sleep(5)
        
        return None
    
    async def _submit_captcha(self, sitekey: str, pageurl: str) -> Optional[str]:
        """Submete captcha para resolução"""
        try:
            response = self.session.post(f"{self.base_url}/in.php", data={
                'key': self.api_key,
                'method': 'hcaptcha',
                'sitekey': sitekey,
                'pageurl': pageurl,
                'json': '1'
            }, timeout=30)
            
            result = response.json()
            if result.get('status') == 1:
                logging.info(f"Captcha submetido: {result['request']}")
                return result['request']
                
        except Exception as e:
            logging.error(f"Erro ao submeter captcha: {e}")
        
        return None
    
    async def _wait_solution(self, captcha_id: str) -> Optional[str]:
        """Aguarda solução do captcha"""
        start_time = time.time()
        
        while time.time() - start_time < self.timeout:
            try:
                response = self.session.get(f"{self.base_url}/res.php", params={
                    'key': self.api_key,
                    'action': 'get',
                    'id': captcha_id,
                    'json': '1'
                }, timeout=30)
                
                result = response.json()
                
                if result.get('status') == 1:
                    logging.info("hCaptcha resolvido!")
                    return result['request']
                elif result.get('request') != 'CAPCHA_NOT_READY':
                    logging.error(f"Erro na resolução: {result.get('request')}")
                    break
                
                await asyncio.sleep(3)
                
            except Exception as e:
                logging.error(f"Erro ao verificar solução: {e}")
                await asyncio.sleep(5)
        
        return None


class CNPJDataExtractor:
    """Extrator otimizado de dados CNPJ"""
    
    @staticmethod
    def extract_from_text(text: str) -> CNPJData:
        """Extrai dados estruturados do texto"""
        lines = [line.strip() for line in text.split('\n') if line.strip()]
        
        # Estrutura de dados inicializada
        data = {
            "cnpj": {"numero": "", "tipo": "", "data_abertura": ""},
            "empresa": {
                "razao_social": "", "nome_fantasia": "", "porte": "",
                "natureza_juridica": {"codigo": "", "descricao": ""}
            },
            "atividades": {
                "principal": {"codigo": "", "descricao": ""},
                "secundarias": []
            },
            "endereco": {
                "logradouro": "", "numero": "", "complemento": "",
                "cep": "", "bairro": "", "municipio": "", "uf": ""
            },
            "contato": {"email": "", "telefone": ""},
            "situacao": {
                "cadastral": "", "data_situacao": "", "motivo": "",
                "situacao_especial": "", "data_situacao_especial": ""
            },
            "comprovante": {"data_emissao": "", "hora_emissao": ""},
            "metadados": {
                "timestamp": time.strftime("%Y-%m-%d %H:%M:%S"),
                "sucesso": True
            }
        }
        
        # Mapeamento de campos para extração otimizada
        field_map = {
            "NÚMERO DE INSCRIÇÃO": ("cnpj", "numero"),
            "DATA DE ABERTURA": ("cnpj", "data_abertura"),
            "NOME EMPRESARIAL": ("empresa", "razao_social"),
            "TÍTULO DO ESTABELECIMENTO (NOME DE FANTASIA)": ("empresa", "nome_fantasia"),
            "PORTE": ("empresa", "porte"),
            "LOGRADOURO": ("endereco", "logradouro"),
            "NÚMERO": ("endereco", "numero"),
            "COMPLEMENTO": ("endereco", "complemento"),
            "CEP": ("endereco", "cep"),
            "BAIRRO/DISTRITO": ("endereco", "bairro"),
            "MUNICÍPIO": ("endereco", "municipio"),
            "UF": ("endereco", "uf"),
            "TELEFONE": ("contato", "telefone"),
            "SITUAÇÃO CADASTRAL": ("situacao", "cadastral"),
            "DATA DA SITUAÇÃO CADASTRAL": ("situacao", "data_situacao"),
        }
        
        # Extração otimizada
        for i, line in enumerate(lines):
            next_line = lines[i + 1] if i + 1 < len(lines) else ""
            
            # Campos simples
            if line in field_map:
                section, field = field_map[line]
                if next_line and next_line not in field_map:
                    if section == "endereco" and field == "numero" and data["endereco"]["numero"]:
                        continue  # Evita sobrescrever número já preenchido
                    data[section][field] = next_line
            
            # Campos especiais
            elif line == "MATRIZ":
                data["cnpj"]["tipo"] = "MATRIZ"
            elif line == "FILIAL":
                data["cnpj"]["tipo"] = "FILIAL"
            elif "CÓDIGO E DESCRIÇÃO DA NATUREZA JURÍDICA" in line and next_line:
                if " - " in next_line:
                    codigo, desc = next_line.split(" - ", 1)
                    data["empresa"]["natureza_juridica"] = {"codigo": codigo, "descricao": desc}
            elif "CÓDIGO E DESCRIÇÃO DA ATIVIDADE ECONÔMICA PRINCIPAL" in line and next_line:
                if " - " in next_line:
                    codigo, desc = next_line.split(" - ", 1)
                    data["atividades"]["principal"] = {"codigo": codigo, "descricao": desc}
            elif "CÓDIGO E DESCRIÇÃO DAS ATIVIDADES ECONÔMICAS SECUNDÁRIAS" in line:
                # Extrai atividades secundárias
                j = i + 1
                while j < len(lines) and " - " in lines[j]:
                    if any(stop in lines[j] for stop in ["NATUREZA JURÍDICA", "LOGRADOURO"]):
                        break
                    codigo, desc = lines[j].split(" - ", 1)
                    data["atividades"]["secundarias"].append({"codigo": codigo, "descricao": desc})
                    j += 1
            elif "Emitido no dia" in line:
                match = re.search(r'(\d{2}/\d{2}/\d{4}) às (\d{2}:\d{2}:\d{2})', line)
                if match:
                    data["comprovante"]["data_emissao"] = match.group(1)
                    data["comprovante"]["hora_emissao"] = match.group(2)
        
        return CNPJData(**data)


class CNPJConsultorV2:
    """Consultor CNPJ Ultra Robusto e Performático"""
    
    def __init__(self, api_key: str, headless: bool = True, cache_dir: str = "cache"):
        self.api_key = api_key
        self.headless = headless
        self.cache_dir = Path(cache_dir)
        self.cache_dir.mkdir(exist_ok=True)
        
        # Componentes
        self.captcha_client = SolveCaptchaClient(api_key)
        self.extractor = CNPJDataExtractor()
        
        # Estado
        self.playwright: Optional[Playwright] = None
        self.browser: Optional[Browser] = None
        self.page: Optional[Page] = None
        
        # Configuração de logging
        self._setup_logging()
    
    def _setup_logging(self):
        """Configura logging otimizado"""
        logging.basicConfig(
            level=logging.INFO,
            format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
            handlers=[
                logging.StreamHandler(),
                logging.FileHandler(self.cache_dir / 'cnpj_consultor.log')
            ]
        )
        self.logger = logging.getLogger(__name__)
    
    async def consultar(self, cnpj: str, use_cache: bool = True) -> Dict:
        """
        Consulta CNPJ com cache e retry automático
        
        Args:
            cnpj: CNPJ a ser consultado
            use_cache: Se deve usar cache
            
        Returns:
            Dados estruturados do CNPJ
        """
        cnpj_clean = self._clean_cnpj(cnpj)
        
        # Verifica cache
        if use_cache:
            cached = self._get_cache(cnpj_clean)
            if cached:
                self.logger.info(f"Dados encontrados no cache para CNPJ: {cnpj_clean}")
                return cached
        
        # Consulta online com retry
        max_retries = 3
        for attempt in range(max_retries):
            try:
                result = await self._perform_consultation(cnpj_clean)
                
                # Salva no cache se bem-sucedido
                if result.get("metadados", {}).get("sucesso") and use_cache:
                    self._save_cache(cnpj_clean, result)
                
                return result
                
            except Exception as e:
                self.logger.error(f"Tentativa {attempt + 1} falhou: {e}")
                if attempt < max_retries - 1:
                    await asyncio.sleep(10)
                else:
                    return self._error_response(str(e))
        
        return self._error_response("Máximo de tentativas excedido")
    
    async def _perform_consultation(self, cnpj: str) -> Dict:
        """Executa consulta completa"""
        try:
            await self._setup_browser()
            
            # Navega para página
            url = f"https://solucoes.receita.fazenda.gov.br/servicos/cnpjreva/Cnpjreva_Solicitacao.asp?cnpj={cnpj}"
            await self.page.goto(url, wait_until='networkidle', timeout=30000)
            
            # Resolve captcha
            await self._solve_captcha()
            
            # Submete consulta
            await self._submit_query()
            
            # Extrai dados
            text = await self.page.locator('body').inner_text()
            data = self.extractor.extract_from_text(text)
            data.metadados["url_consulta"] = self.page.url
            
            return asdict(data)
            
        finally:
            await self._cleanup()
    
    async def _setup_browser(self):
        """Configura navegador otimizado"""
        if not self.playwright:
            self.playwright = await async_playwright().start()
            
            self.browser = await self.playwright.chromium.launch(
                headless=self.headless,
                args=[
                    "--no-sandbox", "--disable-dev-shm-usage", "--disable-gpu",
                    "--disable-extensions", "--disable-web-security",
                    "--disable-features=VizDisplayCompositor",
                    "--disable-background-timer-throttling"
                ]
            )
            
            context = await self.browser.new_context(
                viewport={'width': 1200, 'height': 800},
                user_agent='Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
            )
            
            self.page = await context.new_page()
            
            # Otimizações de performance
            await self.page.route("**/*.{png,jpg,jpeg,gif,svg,css,woff,woff2}", 
                                lambda route: route.abort())
    
    async def _solve_captcha(self):
        """Resolve captcha com timeout otimizado"""
        try:
            element = await self.page.wait_for_selector("[data-sitekey]", timeout=10000)
            if element:
                sitekey = await element.get_attribute("data-sitekey")
                token = await self.captcha_client.solve_hcaptcha(sitekey, self.page.url)
                
                if token:
                    await self.page.evaluate(f"""
                        ['h-captcha-response', 'g-recaptcha-response'].forEach(name => {{
                            const el = document.querySelector(`textarea[name="${{name}}"]`);
                            if (el) el.value = '{token}';
                        }});
                    """)
                    await asyncio.sleep(2)
                else:
                    raise Exception("Falha ao resolver captcha")
        except Exception as e:
            await self._save_debug_info("captcha_error")
            raise e
    
    async def _submit_query(self):
        """Submete consulta com verificação robusta"""
        button = await self.page.wait_for_selector("button.btn-primary", timeout=10000)
        await button.click()
        
        # Aguarda resultado com múltiplas verificações
        try:
            await self.page.wait_for_url("**/Cnpjreva_Comprovante.asp**", timeout=30000)
        except:
            await self.page.wait_for_selector("text=COMPROVANTE DE INSCRIÇÃO", timeout=15000)
    
    async def _save_debug_info(self, error_type: str):
        """Salva informações de debug"""
        if self.page:
            timestamp = time.strftime("%Y%m%d_%H%M%S")
            try:
                await self.page.screenshot(path=self.cache_dir / f"{error_type}_{timestamp}.png")
                html = await self.page.content()
                (self.cache_dir / f"{error_type}_{timestamp}.html").write_text(html, encoding='utf-8')
            except:
                pass
    
    def _clean_cnpj(self, cnpj: str) -> str:
        """Limpa e valida CNPJ"""
        clean = re.sub(r'\D', '', cnpj)
        if len(clean) != 14:
            raise ValueError(f"CNPJ inválido: {cnpj}")
        return clean
    
    def _get_cache(self, cnpj: str) -> Optional[Dict]:
        """Recupera dados do cache"""
        cache_file = self.cache_dir / f"{cnpj}.json"
        if cache_file.exists():
            try:
                data = json.loads(cache_file.read_text(encoding='utf-8'))
                # Verifica se cache não expirou (24h)
                cache_time = time.mktime(time.strptime(
                    data.get("metadados", {}).get("timestamp", ""), "%Y-%m-%d %H:%M:%S"
                ))
                if time.time() - cache_time < 86400:  # 24h
                    return data
            except:
                pass
        return None
    
    def _save_cache(self, cnpj: str, data: Dict):
        """Salva dados no cache"""
        try:
            cache_file = self.cache_dir / f"{cnpj}.json"
            cache_file.write_text(json.dumps(data, ensure_ascii=False, indent=2), encoding='utf-8')
        except Exception as e:
            self.logger.warning(f"Erro ao salvar cache: {e}")
    
    def _error_response(self, error: str) -> Dict:
        """Gera resposta de erro padronizada"""
        return {
            "erro": error,
            "metadados": {
                "sucesso": False,
                "timestamp": time.strftime("%Y-%m-%d %H:%M:%S")
            }
        }
    
    async def _cleanup(self):
        """Limpa recursos"""
        if self.browser:
            await self.browser.close()
            self.browser = None
        if self.playwright:
            await self.playwright.stop()
            self.playwright = None


# Função de conveniência
async def consultar_cnpj(cnpj: str, api_key: str = "bd238cb2bace2dd234e32a8df23486f1") -> Dict:
    """Função simplificada para consulta de CNPJ"""
    consultor = CNPJConsultorV2(api_key)
    return await consultor.consultar(cnpj)


# Exemplo de uso
async def main():
    """Demonstração do sistema"""
    cnpj = "38139407000177"
    
    print("🚀 CNPJ Consultor V2 - Ultra Robusto")
    print(f"📋 Consultando: {cnpj}")
    print("-" * 50)
    
    start_time = time.time()
    resultado = await consultar_cnpj(cnpj)
    elapsed = time.time() - start_time
    
    if resultado.get("metadados", {}).get("sucesso"):
        print(f"✅ Sucesso em {elapsed:.1f}s")
        print(f"🏢 {resultado['empresa']['razao_social']}")
        print(f"📄 {resultado['cnpj']['numero']} ({resultado['cnpj']['tipo']})")
        print(f"📍 {resultado['endereco']['municipio']}/{resultado['endereco']['uf']}")
    else:
        print(f"❌ Erro: {resultado.get('erro', 'Desconhecido')}")
    
    print("\n📊 JSON Completo:")
    print(json.dumps(resultado, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    asyncio.run(main())
