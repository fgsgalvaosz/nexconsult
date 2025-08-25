@echo off
echo === SINTEGRA MA SCRAPER ===
echo.
echo Este script executa o scraper do Sintegra MA com headless=false
echo.
echo Configurações:
echo - Navegador: Visível (headless=false)
echo - CNPJ: 38139407000177 (exemplo do teste)
echo - CAPTCHA: SolveCaptcha API (se configurada) ou manual
echo.

REM Verificar se a API key está configurada
if "%SOLVECAPTCHA_API_KEY%"=="" (
    echo AVISO: SOLVECAPTCHA_API_KEY não configurada
    echo O CAPTCHA precisará ser resolvido manualmente
    echo.
    echo Para configurar a API:
    echo set SOLVECAPTCHA_API_KEY=sua_chave_aqui
    echo.
    pause
) else (
    echo ✓ SOLVECAPTCHA_API_KEY configurada
    echo.
)

echo Iniciando execução...
echo.

REM Executar o scraper
go run main.go

echo.
echo Execução concluída!
pause