"""
Consulta CNPJ na Receita Federal - Sistema Completo
Resolve hCaptcha automaticamente e retorna dados estruturados em JSON
"""

import time
import logging
import asyncio
import requests
import json
import re
from playwright.async_api import async_playwright


class SolveCaptchaAPI:
    """Cliente para a API do SolveCaptcha.com"""
    
    def __init__(self, api_key):
        self.api_key = api_key
        self.base_url = "https://api.solvecaptcha.com"
        
    def solve_hcaptcha(self, sitekey, pageurl):
        """Resolve hCaptcha usando a API do SolveCaptcha"""
        try:
            submit_url = f"{self.base_url}/in.php"
            submit_data = {
                'key': self.api_key,
                'method': 'hcaptcha',
                'sitekey': sitekey,
                'pageurl': pageurl,
                'json': '1'
            }
            
            response = requests.post(submit_url, data=submit_data)
            result = response.json()
            
            if result.get('status') != 1:
                raise Exception(f"Erro ao submeter captcha: {result.get('request', 'Erro desconhecido')}")
            
            captcha_id = result['request']
            print(f"Captcha submetido com ID: {captcha_id}")
            
            return self._wait_for_result(captcha_id)
            
        except Exception as e:
            print(f"Erro ao resolver hCaptcha: {str(e)}")
            return None
    
    def _wait_for_result(self, captcha_id, max_attempts=60):
        """Aguarda o resultado do captcha"""
        result_url = f"{self.base_url}/res.php"
        
        for attempt in range(max_attempts):
            try:
                response = requests.get(result_url, params={
                    'key': self.api_key,
                    'action': 'get',
                    'id': captcha_id,
                    'json': '1'
                })
                
                result = response.json()
                
                if result.get('status') == 1:
                    print("hCaptcha resolvido com sucesso!")
                    return {'token': result['request']}
                elif result.get('request') == 'CAPCHA_NOT_READY':
                    print(f"Aguardando resolução... ({attempt + 1}/{max_attempts})")
                    time.sleep(5)
                else:
                    raise Exception(f"Erro na resolução: {result.get('request', 'Erro desconhecido')}")
                    
            except Exception as e:
                print(f"Erro ao verificar resultado: {str(e)}")
                time.sleep(5)
        
        return None


class CNPJConsultor:
    """Consultor completo de CNPJ da Receita Federal"""
    
    def __init__(self, solvecaptcha_api_key, headless=True):
        self.solvecaptcha = SolveCaptchaAPI(solvecaptcha_api_key)
        self.headless = headless
        self.playwright = None
        self.browser = None
        self.page = None
        self.setup_logging()
        
    def setup_logging(self):
        """Configura o sistema de logging"""
        logging.basicConfig(
            level=logging.INFO,
            format='%(asctime)s - %(levelname)s - %(message)s'
        )
        self.logger = logging.getLogger(__name__)
        
    async def consultar_cnpj(self, cnpj):
        """
        Consulta um CNPJ e retorna dados estruturados
        
        Args:
            cnpj (str): CNPJ a ser consultado
            
        Returns:
            dict: Dados estruturados do CNPJ
        """
        try:
            # Configura o navegador
            await self._setup_browser()
            
            # Remove caracteres não numéricos do CNPJ
            cnpj_limpo = ''.join(filter(str.isdigit, cnpj))
            
            if len(cnpj_limpo) != 14:
                raise ValueError("CNPJ deve conter exatamente 14 dígitos")
            
            self.logger.info(f"Iniciando consulta para CNPJ: {cnpj_limpo}")
            
            # Navega para a página de consulta
            url = f"https://solucoes.receita.fazenda.gov.br/servicos/cnpjreva/Cnpjreva_Solicitacao.asp?cnpj={cnpj_limpo}"
            await self.page.goto(url)
            await self.page.wait_for_load_state('networkidle')
            await asyncio.sleep(3)
            
            # Resolve hCaptcha
            await self._resolver_captcha()
            
            # Clica no botão consultar
            await self._clicar_consultar()
            
            # Extrai dados
            dados = await self._extrair_dados()
            
            self.logger.info("Consulta realizada com sucesso")
            return dados
            
        except Exception as e:
            # Captura screenshot em caso de erro geral
            await self._screenshot_erro("erro_consulta_geral")
            self.logger.error(f"Erro durante a consulta: {str(e)}")
            return {
                "erro": str(e),
                "sucesso": False,
                "timestamp": time.strftime("%Y-%m-%d %H:%M:%S")
            }
        finally:
            await self._fechar_browser()
    
    async def _setup_browser(self):
        """Configura o navegador"""
        self.playwright = await async_playwright().start()
        
        browser_args = [
            "--no-sandbox",
            "--disable-dev-shm-usage",
            "--disable-gpu",
            "--disable-extensions",
            "--disable-web-security"
        ]
        
        self.browser = await self.playwright.chromium.launch(
            headless=self.headless,
            args=browser_args
        )
        
        context = await self.browser.new_context(
            viewport={'width': 1100, 'height': 633}
        )
        
        self.page = await context.new_page()
        self.logger.info("Navegador configurado")
    
    async def _resolver_captcha(self):
        """Resolve o hCaptcha"""
        try:
            hcaptcha_element = await self.page.query_selector("[data-sitekey]")
            
            if hcaptcha_element:
                self.logger.info("hCaptcha detectado, resolvendo...")
                
                sitekey = await hcaptcha_element.get_attribute("data-sitekey")
                pageurl = self.page.url
                
                result = self.solvecaptcha.solve_hcaptcha(sitekey, pageurl)
                
                if result:
                    await self._aplicar_token(result['token'])
                    self.logger.info("hCaptcha resolvido!")
                else:
                    raise Exception("Falha ao resolver hCaptcha")
            else:
                self.logger.info("Nenhum hCaptcha detectado")
                
        except Exception as e:
            # Captura screenshot em caso de erro no captcha
            await self._screenshot_erro("erro_resolver_captcha")
            self.logger.error(f"Erro ao resolver hCaptcha: {str(e)}")
            raise
    
    async def _aplicar_token(self, token):
        """Aplica o token do hCaptcha"""
        await self.page.evaluate(f"""
            let hcaptchaResponse = document.querySelector('textarea[name="h-captcha-response"]');
            if (hcaptchaResponse) {{
                hcaptchaResponse.value = '{token}';
            }}
            
            let grecaptchaResponse = document.querySelector('textarea[name="g-recaptcha-response"]');
            if (grecaptchaResponse) {{
                grecaptchaResponse.value = '{token}';
            }}
        """)
        await asyncio.sleep(2)
    
    async def _clicar_consultar(self):
        """Clica no botão CONSULTAR"""
        try:
            botao = await self.page.wait_for_selector("button.btn-primary", timeout=10000)
            await botao.click()
            self.logger.info("Botão CONSULTAR clicado")

            # Aguarda a navegação com timeout maior e múltiplas tentativas
            try:
                await self.page.wait_for_url("**/Cnpjreva_Comprovante.asp**", timeout=30000)
                self.logger.info("Navegação para página de comprovante bem-sucedida")
            except:
                self.logger.warning("Timeout na navegação por URL, tentando por conteúdo...")
                try:
                    # Se não conseguir pela URL, tenta aguardar pelo conteúdo da página
                    await self.page.wait_for_selector("text=COMPROVANTE DE INSCRIÇÃO", timeout=15000)
                    self.logger.info("Página de comprovante detectada por conteúdo")
                except:
                    # Última tentativa: verifica se há algum erro na página
                    current_url = self.page.url
                    self.logger.error(f"Falha ao carregar página de resultado. URL atual: {current_url}")

                    # Verifica se há mensagens de erro na página
                    try:
                        error_elements = await self.page.query_selector_all("text=/erro|error|falha|inválido/i")
                        if error_elements:
                            error_text = await error_elements[0].inner_text()
                            self.logger.error(f"Erro detectado na página: {error_text}")
                    except:
                        pass

                    raise Exception("Timeout ao aguardar página de resultado")

        except Exception as e:
            # Captura screenshot em caso de erro
            await self._screenshot_erro("erro_clicar_consultar")
            self.logger.error(f"Erro ao clicar no botão: {str(e)}")
            raise
    
    async def _extrair_dados(self):
        """Extrai dados estruturados da página de resultado"""
        try:
            await self.page.wait_for_load_state('networkidle')
            await asyncio.sleep(2)
            
            # Extrai texto da página
            texto = await self.page.locator('body').inner_text()
            
            # Processa e estrutura os dados
            dados = self._processar_dados(texto)
            
            # Adiciona metadados
            dados["metadados"] = {
                "url_consulta": self.page.url,
                "timestamp": time.strftime("%Y-%m-%d %H:%M:%S"),
                "sucesso": True
            }
            
            return dados
            
        except Exception as e:
            self.logger.error(f"Erro ao extrair dados: {str(e)}")
            return {
                "erro": str(e),
                "sucesso": False,
                "timestamp": time.strftime("%Y-%m-%d %H:%M:%S")
            }
    
    def _processar_dados(self, texto):
        """Processa o texto e extrai dados estruturados"""
        linhas = texto.split('\n')
        
        dados = {
            "cnpj": {"numero": "", "tipo": "", "data_abertura": ""},
            "empresa": {
                "razao_social": "",
                "nome_fantasia": "",
                "porte": "",
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
                "cadastral": "", "data_situacao": "",
                "motivo": "", "situacao_especial": "", "data_situacao_especial": ""
            },
            "comprovante": {"data_emissao": "", "hora_emissao": ""}
        }
        
        for i, linha in enumerate(linhas):
            linha = linha.strip()
            
            # Função auxiliar para pegar próxima linha não vazia
            def proxima_linha(idx):
                j = idx + 1
                while j < len(linhas) and not linhas[j].strip():
                    j += 1
                return linhas[j].strip() if j < len(linhas) else ""
            
            # CNPJ
            if "NÚMERO DE INSCRIÇÃO" in linha:
                dados["cnpj"]["numero"] = proxima_linha(i)
                if i + 2 < len(linhas) and "MATRIZ" in linhas[i + 2]:
                    dados["cnpj"]["tipo"] = "MATRIZ"
                elif i + 2 < len(linhas) and "FILIAL" in linhas[i + 2]:
                    dados["cnpj"]["tipo"] = "FILIAL"
            
            elif "DATA DE ABERTURA" in linha:
                dados["cnpj"]["data_abertura"] = proxima_linha(i)
            
            # Empresa
            elif "NOME EMPRESARIAL" in linha:
                dados["empresa"]["razao_social"] = proxima_linha(i)
            
            elif "TÍTULO DO ESTABELECIMENTO (NOME DE FANTASIA)" in linha:
                dados["empresa"]["nome_fantasia"] = proxima_linha(i)
            
            elif "PORTE" in linha and len(linha) < 20:
                dados["empresa"]["porte"] = proxima_linha(i)
            
            elif "CÓDIGO E DESCRIÇÃO DA NATUREZA JURÍDICA" in linha:
                natureza = proxima_linha(i)
                if " - " in natureza:
                    codigo, descricao = natureza.split(" - ", 1)
                    dados["empresa"]["natureza_juridica"]["codigo"] = codigo
                    dados["empresa"]["natureza_juridica"]["descricao"] = descricao
            
            # Atividade principal
            elif "CÓDIGO E DESCRIÇÃO DA ATIVIDADE ECONÔMICA PRINCIPAL" in linha:
                atividade = proxima_linha(i)
                if " - " in atividade:
                    codigo, descricao = atividade.split(" - ", 1)
                    dados["atividades"]["principal"]["codigo"] = codigo
                    dados["atividades"]["principal"]["descricao"] = descricao

            # Atividades secundárias
            elif "CÓDIGO E DESCRIÇÃO DAS ATIVIDADES ECONÔMICAS SECUNDÁRIAS" in linha:
                j = i + 1
                while j < len(linhas):
                    linha_atividade = linhas[j].strip()
                    if not linha_atividade:
                        j += 1
                        continue
                    if any(campo in linha_atividade for campo in ["CÓDIGO E DESCRIÇÃO", "LOGRADOURO", "NATUREZA JURÍDICA"]):
                        break
                    if " - " in linha_atividade:
                        codigo, descricao = linha_atividade.split(" - ", 1)
                        dados["atividades"]["secundarias"].append({
                            "codigo": codigo,
                            "descricao": descricao
                        })
                    j += 1
            
            # Endereço
            elif "LOGRADOURO" in linha:
                dados["endereco"]["logradouro"] = proxima_linha(i)
            elif "NÚMERO" in linha and not dados["endereco"]["numero"]:
                dados["endereco"]["numero"] = proxima_linha(i)
            elif "BAIRRO/DISTRITO" in linha:
                dados["endereco"]["bairro"] = proxima_linha(i)
            elif "MUNICÍPIO" in linha:
                dados["endereco"]["municipio"] = proxima_linha(i)
            elif "UF" in linha and len(linha) < 10:
                dados["endereco"]["uf"] = proxima_linha(i)
            elif "CEP" in linha:
                dados["endereco"]["cep"] = proxima_linha(i)
            
            # Contato
            elif "TELEFONE" in linha:
                telefone = proxima_linha(i)
                if telefone and telefone != "********":
                    dados["contato"]["telefone"] = telefone
            
            # Situação Cadastral
            elif linha == "SITUAÇÃO CADASTRAL":
                situacao = proxima_linha(i)
                if situacao and situacao not in ["DATA DA SITUAÇÃO CADASTRAL", "MOTIVO"]:
                    dados["situacao"]["cadastral"] = situacao

            elif "DATA DA SITUAÇÃO CADASTRAL" in linha:
                data_situacao = proxima_linha(i)
                if data_situacao and "/" in data_situacao:
                    dados["situacao"]["data_situacao"] = data_situacao

            elif "MOTIVO DE SITUAÇÃO CADASTRAL" in linha:
                motivo = proxima_linha(i)
                if motivo and motivo != "********" and motivo.strip():
                    dados["situacao"]["motivo"] = motivo

            elif linha == "SITUAÇÃO ESPECIAL":
                situacao_especial = proxima_linha(i)
                if situacao_especial and situacao_especial != "********" and situacao_especial.strip():
                    dados["situacao"]["situacao_especial"] = situacao_especial

            elif "DATA DA SITUAÇÃO ESPECIAL" in linha:
                data_especial = proxima_linha(i)
                if data_especial and data_especial != "********" and "/" in data_especial:
                    dados["situacao"]["data_situacao_especial"] = data_especial
            
            # Data de emissão
            elif "Emitido no dia" in linha:
                match = re.search(r'(\d{2}/\d{2}/\d{4}) às (\d{2}:\d{2}:\d{2})', linha)
                if match:
                    dados["comprovante"]["data_emissao"] = match.group(1)
                    dados["comprovante"]["hora_emissao"] = match.group(2)
        
        return dados
    
    async def _screenshot_erro(self, nome_erro):
        """Captura screenshot em caso de erro"""
        try:
            if self.page:
                timestamp = time.strftime("%Y%m%d_%H%M%S")
                filename = f"erro_{nome_erro}_{timestamp}.png"
                await self.page.screenshot(path=filename, full_page=True)
                self.logger.info(f"📸 Screenshot de erro salvo: {filename}")

                # Também salva o HTML da página atual
                html_filename = f"erro_{nome_erro}_{timestamp}.html"
                html_content = await self.page.content()
                with open(html_filename, 'w', encoding='utf-8') as f:
                    f.write(html_content)
                self.logger.info(f"📄 HTML de erro salvo: {html_filename}")

        except Exception as e:
            self.logger.warning(f"Erro ao capturar screenshot: {str(e)}")

    async def _fechar_browser(self):
        """Fecha o navegador"""
        if self.browser:
            await self.browser.close()
        if self.playwright:
            await self.playwright.stop()


async def consultar_cnpj_simples(cnpj, api_key="bd238cb2bace2dd234e32a8df23486f1"):
    """
    Função simplificada para consultar CNPJ

    Args:
        cnpj (str): CNPJ a ser consultado
        api_key (str): Chave da API do SolveCaptcha

    Returns:
        dict: Dados estruturados do CNPJ
    """
    consultor = CNPJConsultor(solvecaptcha_api_key=api_key, headless=True)
    return await consultor.consultar_cnpj(cnpj)


async def main():
    """Função principal para teste"""
    # Configuração
    API_KEY = "bd238cb2bace2dd234e32a8df23486f1"
    CNPJ_TESTE = "38139407000177"

    print("🔍 Iniciando consulta CNPJ...")
    print(f"📋 CNPJ: {CNPJ_TESTE}")
    print("-" * 50)

    # Realiza a consulta
    resultado = await consultar_cnpj_simples(CNPJ_TESTE, API_KEY)

    # Exibe o resultado
    if resultado.get("metadados", {}).get("sucesso"):
        print("\n✅ CONSULTA REALIZADA COM SUCESSO!")
        print("="*80)

        # Dados principais
        cnpj_data = resultado.get("cnpj", {})
        empresa_data = resultado.get("empresa", {})
        endereco_data = resultado.get("endereco", {})
        situacao_data = resultado.get("situacao", {})

        print(f"🏢 EMPRESA: {empresa_data.get('razao_social', 'N/A')}")
        print(f"🏷️  FANTASIA: {empresa_data.get('nome_fantasia', 'N/A')}")
        print(f"📄 CNPJ: {cnpj_data.get('numero', 'N/A')} ({cnpj_data.get('tipo', 'N/A')})")
        print(f"📅 ABERTURA: {cnpj_data.get('data_abertura', 'N/A')}")
        print(f"📊 PORTE: {empresa_data.get('porte', 'N/A')}")
        print(f"✅ SITUAÇÃO: {situacao_data.get('cadastral', 'N/A')}")
        print(f"📍 ENDEREÇO: {endereco_data.get('logradouro', 'N/A')}, {endereco_data.get('numero', 'N/A')}")
        print(f"🏙️  CIDADE: {endereco_data.get('municipio', 'N/A')}/{endereco_data.get('uf', 'N/A')}")
        print(f"📞 TELEFONE: {resultado.get('contato', {}).get('telefone', 'N/A')}")

        atividade_principal = resultado.get("atividades", {}).get("principal", {})
        if atividade_principal.get("codigo"):
            print(f"🎯 ATIVIDADE: {atividade_principal.get('codigo')} - {atividade_principal.get('descricao')}")

        secundarias = resultado.get("atividades", {}).get("secundarias", [])
        if secundarias:
            print(f"📋 ATIVIDADES SECUNDÁRIAS: {len(secundarias)} atividades")

        print("="*80)
        print("\n📄 JSON COMPLETO:")
        print(json.dumps(resultado, ensure_ascii=False, indent=2))

    else:
        print("❌ ERRO NA CONSULTA:")
        print(json.dumps(resultado, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    asyncio.run(main())
