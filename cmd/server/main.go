// @title NexConsult Sintegra MA API
// @version 1.0.0
// @description API REST para consulta automatizada no Sintegra do Maranhão com resolução automática de reCAPTCHA v2 e extração estruturada de dados.
// @termsOfService http://swagger.io/terms/
// @contact.name NexConsult
// @contact.email contato@nexconsult.com
// @contact.url https://nexconsult.com
// @license.name Apache 2.0
// @license.url http://www.apache.org/licenses/LICENSE-2.0.html
// @host localhost:3000
// @BasePath /
// @schemes http https
package main

import (
	"fmt"
	"nexconsult-sintegra-ma/internal/api/middleware"
	"nexconsult-sintegra-ma/internal/api/router"
	"nexconsult-sintegra-ma/internal/logger"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

func main() {
	// Configurar logger estruturado
	appLogger := logger.Setup()
	appLogger.Info().Msg("=== NEXCONSULT SINTEGRA MA API ===")
	appLogger.Info().Msg("Inicializando servidor...")

	// Configurar porta
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	// Configurar host
	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	// Configurar aplicação Fiber
	app := setupFiberApp()

	// Configurar rotas
	router.SetupRoutes(app, appLogger)

	// Log de inicialização
	appLogger.Info().
		Str("host", host).
		Str("port", port).
		Msg("🚀 Servidor configurado")

	// Configurar graceful shutdown
	go func() {
		if err := app.Listen(fmt.Sprintf("%s:%s", host, port)); err != nil {
			appLogger.Fatal().Err(err).Msg("❌ Erro ao iniciar servidor")
		}
	}()

	appLogger.Info().
		Str("address", fmt.Sprintf("http://%s:%s", host, port)).
		Msg("✅ Servidor rodando")

	// Imprimir informações úteis
	printServerInfo(host, port)

	// Aguardar sinal de shutdown
	waitForShutdown(app, appLogger)
}

// setupFiberApp configura a aplicação Fiber com settings otimizados
func setupFiberApp() *fiber.App {
	return fiber.New(fiber.Config{
		// Configurações de performance
		Prefork:       false, // Não usar prefork em desenvolvimento
		CaseSensitive: true,
		StrictRouting: false,
		ServerHeader:  "NexConsult-Sintegra-MA-API",
		AppName:       "NexConsult Sintegra MA API v1.0.0",

		// Configurações de limite
		BodyLimit: 4 * 1024 * 1024, // 4MB

		// Configurações de timeout
		ReadTimeout:  60 * 1000,  // 60 segundos em milissegundos
		WriteTimeout: 60 * 1000,  // 60 segundos em milissegundos
		IdleTimeout:  120 * 1000, // 120 segundos em milissegundos

		// Error handler personalizado
		ErrorHandler: middleware.ErrorHandler(),

		// Desabilitar banner do Fiber
		DisableStartupMessage: true,
	})
}

// printServerInfo imprime informações úteis sobre o servidor
func printServerInfo(host, port string) {
	fmt.Println("╔════════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                      🚀 NEXCONSULT SINTEGRA MA API 🚀                          ║")
	fmt.Println("╠════════════════════════════════════════════════════════════════════════════════╣")
	
	localAddr := fmt.Sprintf("http://localhost:%s", port)
	if host != "127.0.0.1" && host != "localhost" {
		fmt.Printf("║ 🌐 Servidor:     http://%s:%s%s║\n", host, port, getSpacing(fmt.Sprintf("http://%s:%s", host, port)))
	}
	fmt.Printf("║ 🏠 Local:        %s%s║\n", localAddr, getSpacing(localAddr))
	fmt.Println("╠════════════════════════════════════════════════════════════════════════════════╣")
	fmt.Printf("║ 📊 Health:       %s/health%s║\n", localAddr, getSpacing(localAddr+"/health"))
	fmt.Printf("║ 📚 Docs:         %s/docs%s║\n", localAddr, getSpacing(localAddr+"/docs"))
	fmt.Printf("║ 📖 Swagger:      %s/swagger/%s║\n", localAddr, getSpacing(localAddr+"/swagger/"))
	fmt.Printf("║ 🔍 Consulta:     %s/api/v1/sintegra/consultar%s║\n", localAddr, getSpacing(localAddr+"/api/v1/sintegra/consultar"))
	fmt.Println("╠════════════════════════════════════════════════════════════════════════════════╣")
	fmt.Println("║ 💡 Exemplo POST: curl -X POST http://localhost:3000/api/v1/sintegra/consultar ║")
	fmt.Println("║                  -H \"Content-Type: application/json\"                          ║")
	fmt.Println("║                  -d '{\"cnpj\": \"38139407000177\"}'                             ║")
	fmt.Println("║                                                                                ║")
	fmt.Println("║ 💡 Exemplo GET:  curl http://localhost:3000/api/v1/sintegra/consultar/38139407000177 ║")
	fmt.Println("╠════════════════════════════════════════════════════════════════════════════════╣")
	fmt.Println("║ ⚡ Rate Limit:   10 req/min por IP                                             ║")
	fmt.Println("║ 🔒 CORS:         Habilitado para desenvolvimento                              ║")
	fmt.Println("║ 📝 Logs:         Estruturados com zerolog                                     ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("🛑 Pressione Ctrl+C para parar o servidor")
	fmt.Println()
}

// getSpacing calcula espaçamento para formatação da tabela
func getSpacing(text string) string {
	maxWidth := 62 // Largura máxima da linha
	spaces := maxWidth - len(text)
	if spaces < 0 {
		spaces = 0
	}
	result := ""
	for i := 0; i < spaces; i++ {
		result += " "
	}
	return result
}

// waitForShutdown aguarda sinal de shutdown e encerra gracefully
func waitForShutdown(app *fiber.App, appLogger zerolog.Logger) {
	// Criar channel para capturar sinais do OS
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	// Aguardar sinal
	<-quit
	
	appLogger.Info().Msg("🛑 Sinal de shutdown recebido...")

	// Encerrar aplicação
	if err := app.Shutdown(); err != nil {
		appLogger.Error().Err(err).Msg("❌ Erro durante shutdown")
	} else {
		appLogger.Info().Msg("✅ Servidor encerrado gracefully")
	}
}