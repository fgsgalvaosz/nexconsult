"""
Automação para consulta de CNPJ na Receita Federal
Baseado nos passos de gravação fornecidos
"""

import time
import logging
from selenium import webdriver
from selenium.webdriver.common.by import By
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.support import expected_conditions as EC
from selenium.webdriver.chrome.service import Service
from selenium.webdriver.chrome.options import Options
from webdriver_manager.chrome import ChromeDriverManager
from selenium.common.exceptions import TimeoutException, NoSuchElementException


class CNPJConsulta:
    def __init__(self, headless=False):
        """
        Inicializa o consultor de CNPJ
        
        Args:
            headless (bool): Se True, executa o navegador em modo headless
        """
        self.driver = None
        self.headless = headless
        self.setup_logging()
        
    def setup_logging(self):
        """Configura o sistema de logging"""
        logging.basicConfig(
            level=logging.INFO,
            format='%(asctime)s - %(levelname)s - %(message)s'
        )
        self.logger = logging.getLogger(__name__)
        
    def setup_driver(self):
        """Configura e inicializa o driver do Chrome"""
        try:
            chrome_options = Options()

            if self.headless:
                chrome_options.add_argument("--headless")

            # Configurações para melhor compatibilidade em ambientes Linux/Docker
            chrome_options.add_argument("--no-sandbox")
            chrome_options.add_argument("--disable-dev-shm-usage")
            chrome_options.add_argument("--disable-gpu")
            chrome_options.add_argument("--disable-extensions")
            chrome_options.add_argument("--disable-plugins")
            chrome_options.add_argument("--disable-images")
            chrome_options.add_argument("--disable-javascript")
            chrome_options.add_argument("--window-size=1100,633")
            chrome_options.add_argument("--remote-debugging-port=9222")

            # Tenta diferentes abordagens para configurar o driver
            service = None

            try:
                # Primeira tentativa: usar webdriver-manager
                driver_path = ChromeDriverManager().install()

                # Verifica se o arquivo baixado é executável
                import os
                import stat

                # Procura pelo arquivo chromedriver correto
                if os.path.isdir(driver_path):
                    # Se é um diretório, procura pelo executável
                    for root, _, files in os.walk(driver_path):
                        for file in files:
                            if file == 'chromedriver' and os.access(os.path.join(root, file), os.X_OK):
                                driver_path = os.path.join(root, file)
                                break

                # Torna o arquivo executável se necessário
                if os.path.isfile(driver_path):
                    os.chmod(driver_path, stat.S_IRWXU | stat.S_IRGRP | stat.S_IXGRP | stat.S_IROTH | stat.S_IXOTH)
                    service = Service(driver_path)
                    self.logger.info(f"Usando ChromeDriver em: {driver_path}")

            except Exception as e:
                self.logger.warning(f"Falha ao usar webdriver-manager: {str(e)}")

            # Segunda tentativa: usar chromedriver do sistema
            if service is None:
                try:
                    service = Service()  # Usa o chromedriver do PATH
                    self.logger.info("Usando ChromeDriver do sistema (PATH)")
                except Exception as e:
                    self.logger.warning(f"ChromeDriver não encontrado no PATH: {str(e)}")

            # Terceira tentativa: sem service (deixa o Selenium gerenciar)
            if service is None:
                self.logger.info("Tentando inicializar Chrome sem service específico")
                self.driver = webdriver.Chrome(options=chrome_options)
            else:
                self.driver = webdriver.Chrome(service=service, options=chrome_options)

            self.driver.set_window_size(1100, 633)
            self.logger.info("Driver do Chrome configurado com sucesso")

        except Exception as e:
            self.logger.error(f"Erro ao configurar o driver: {str(e)}")
            self.logger.info("Tentando instalar ChromeDriver manualmente...")
            self._install_chromedriver_manual()
            raise
            
    def consultar_cnpj(self, cnpj):
        """
        Consulta um CNPJ na Receita Federal
        
        Args:
            cnpj (str): CNPJ a ser consultado (apenas números)
            
        Returns:
            dict: Resultado da consulta ou None em caso de erro
        """
        if not self.driver:
            self.setup_driver()
            
        try:
            # Remove caracteres não numéricos do CNPJ
            cnpj_limpo = ''.join(filter(str.isdigit, cnpj))
            
            if len(cnpj_limpo) != 14:
                raise ValueError("CNPJ deve conter exatamente 14 dígitos")
            
            self.logger.info(f"Iniciando consulta para CNPJ: {cnpj_limpo}")
            
            # Navega para a URL da consulta
            url = f"https://solucoes.receita.fazenda.gov.br/servicos/cnpjreva/Cnpjreva_Solicitacao.asp?cnpj={cnpj_limpo}"
            self.driver.get(url)
            
            self.logger.info("Página carregada, aguardando elementos...")
            
            # Aguarda a página carregar completamente
            WebDriverWait(self.driver, 10).until(
                EC.presence_of_element_located((By.TAG_NAME, "body"))
            )
            
            # Aguarda um pouco para garantir que todos os elementos estejam carregados
            time.sleep(3)
            
            # Tenta resolver o hCaptcha (se presente)
            self._resolver_captcha()
            
            # Clica no botão CONSULTAR
            self._clicar_consultar()
            
            # Aguarda o resultado e extrai os dados
            resultado = self._extrair_dados_resultado()
            
            self.logger.info("Consulta realizada com sucesso")
            return resultado
            
        except Exception as e:
            self.logger.error(f"Erro durante a consulta: {str(e)}")
            return None
            
    def _resolver_captcha(self):
        """
        Tenta resolver o hCaptcha automaticamente
        Nota: Em um ambiente real, isso pode requerer intervenção manual
        """
        try:
            # Procura pelo iframe do hCaptcha
            captcha_frames = self.driver.find_elements(By.CSS_SELECTOR, "iframe[src*='hcaptcha']")
            
            if captcha_frames:
                self.logger.info("hCaptcha detectado, tentando resolver...")
                
                # Muda para o iframe do captcha
                self.driver.switch_to.frame(captcha_frames[0])
                
                # Procura pelo checkbox
                checkbox = WebDriverWait(self.driver, 10).until(
                    EC.element_to_be_clickable((By.ID, "checkbox"))
                )
                
                # Clica no checkbox
                checkbox.click()
                self.logger.info("Checkbox do hCaptcha clicado")
                
                # Volta para o frame principal
                self.driver.switch_to.default_content()
                
                # Aguarda um pouco para o captcha processar
                time.sleep(3)
                
        except (TimeoutException, NoSuchElementException):
            self.logger.warning("hCaptcha não encontrado ou não foi possível resolver")
            
    def _clicar_consultar(self):
        """Clica no botão CONSULTAR"""
        try:
            # Procura pelo botão CONSULTAR
            botao_consultar = WebDriverWait(self.driver, 10).until(
                EC.element_to_be_clickable((By.CSS_SELECTOR, "button.btn-primary"))
            )
            
            botao_consultar.click()
            self.logger.info("Botão CONSULTAR clicado")
            
            # Aguarda a navegação para a página de resultado
            WebDriverWait(self.driver, 15).until(
                EC.url_contains("Cnpjreva_Comprovante.asp")
            )
            
        except TimeoutException:
            self.logger.error("Timeout ao tentar clicar no botão CONSULTAR")
            raise
            
    def _extrair_dados_resultado(self):
        """
        Extrai os dados do resultado da consulta
        
        Returns:
            dict: Dados extraídos da página de resultado
        """
        try:
            # Aguarda a página de resultado carregar
            WebDriverWait(self.driver, 10).until(
                EC.presence_of_element_located((By.TAG_NAME, "body"))
            )
            
            # Aguarda um pouco para garantir que o conteúdo esteja carregado
            time.sleep(2)
            
            # Extrai o HTML da página para análise
            html_content = self.driver.page_source
            
            # Aqui você pode implementar a extração específica dos dados
            # Por enquanto, retorna informações básicas
            resultado = {
                "url_resultado": self.driver.current_url,
                "titulo_pagina": self.driver.title,
                "html_content": html_content,
                "timestamp": time.strftime("%Y-%m-%d %H:%M:%S")
            }
            
            return resultado
            
        except Exception as e:
            self.logger.error(f"Erro ao extrair dados do resultado: {str(e)}")
            return None
            
    def fechar(self):
        """Fecha o navegador"""
        if self.driver:
            self.driver.quit()
            self.logger.info("Navegador fechado")


def main():
    """Função principal para teste"""
    consultor = CNPJConsulta(headless=False)
    
    try:
        # Exemplo de uso com o CNPJ fornecido
        cnpj_teste = "38139407000177"
        resultado = consultor.consultar_cnpj(cnpj_teste)
        
        if resultado:
            print("Consulta realizada com sucesso!")
            print(f"URL do resultado: {resultado['url_resultado']}")
            print(f"Título da página: {resultado['titulo_pagina']}")
        else:
            print("Falha na consulta")
            
    finally:
        consultor.fechar()


if __name__ == "__main__":
    main()
