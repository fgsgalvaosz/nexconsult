"""
Exemplo de uso da classe CNPJConsulta
"""

from cnpj_consulta import CNPJConsulta
import json


def exemplo_consulta_simples():
    """Exemplo básico de consulta"""
    print("=== Exemplo de Consulta Simples ===")
    
    consultor = CNPJConsulta(headless=False)
    
    try:
        # CNPJ de exemplo (mesmo da gravação)
        cnpj = "38139407000177"
        
        print(f"Consultando CNPJ: {cnpj}")
        resultado = consultor.consultar_cnpj(cnpj)
        
        if resultado:
            print("✅ Consulta realizada com sucesso!")
            print(f"📄 Título da página: {resultado['titulo_pagina']}")
            print(f"🔗 URL do resultado: {resultado['url_resultado']}")
            print(f"⏰ Timestamp: {resultado['timestamp']}")
            
            # Salva o HTML do resultado para análise posterior
            with open(f"resultado_{cnpj}.html", "w", encoding="utf-8") as f:
                f.write(resultado['html_content'])
            print(f"💾 HTML salvo em: resultado_{cnpj}.html")
            
        else:
            print("❌ Falha na consulta")
            
    except Exception as e:
        print(f"❌ Erro durante a execução: {str(e)}")
        
    finally:
        consultor.fechar()


def exemplo_multiplas_consultas():
    """Exemplo de múltiplas consultas"""
    print("\n=== Exemplo de Múltiplas Consultas ===")
    
    # Lista de CNPJs para consultar
    cnpjs = [
        "38139407000177",  # CNPJ da gravação
        # Adicione outros CNPJs aqui se necessário
    ]
    
    consultor = CNPJConsulta(headless=True)  # Modo headless para múltiplas consultas
    resultados = []
    
    try:
        for cnpj in cnpjs:
            print(f"Consultando CNPJ: {cnpj}")
            
            resultado = consultor.consultar_cnpj(cnpj)
            
            if resultado:
                print(f"✅ Sucesso para {cnpj}")
                resultados.append({
                    "cnpj": cnpj,
                    "sucesso": True,
                    "url": resultado['url_resultado'],
                    "timestamp": resultado['timestamp']
                })
            else:
                print(f"❌ Falha para {cnpj}")
                resultados.append({
                    "cnpj": cnpj,
                    "sucesso": False,
                    "timestamp": None
                })
            
            # Pausa entre consultas para evitar sobrecarga
            import time
            time.sleep(2)
        
        # Salva resumo dos resultados
        with open("resumo_consultas.json", "w", encoding="utf-8") as f:
            json.dump(resultados, f, indent=2, ensure_ascii=False)
        
        print(f"📊 Resumo salvo em: resumo_consultas.json")
        
    finally:
        consultor.fechar()


def exemplo_com_tratamento_erro():
    """Exemplo com tratamento robusto de erros"""
    print("\n=== Exemplo com Tratamento de Erros ===")
    
    consultor = CNPJConsulta(headless=False)
    
    try:
        # Testa com CNPJ inválido
        cnpj_invalido = "123456789"
        print(f"Testando CNPJ inválido: {cnpj_invalido}")
        
        resultado = consultor.consultar_cnpj(cnpj_invalido)
        
        if resultado:
            print("✅ Consulta realizada")
        else:
            print("❌ Consulta falhou (esperado para CNPJ inválido)")
            
    except ValueError as e:
        print(f"⚠️ Erro de validação: {str(e)}")
        
    except Exception as e:
        print(f"❌ Erro inesperado: {str(e)}")
        
    finally:
        consultor.fechar()


if __name__ == "__main__":
    # Executa os exemplos
    exemplo_consulta_simples()
    exemplo_multiplas_consultas()
    exemplo_com_tratamento_erro()
    
    print("\n🎉 Todos os exemplos executados!")
