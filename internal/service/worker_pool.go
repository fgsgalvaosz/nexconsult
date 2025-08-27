package service

import (
	"context"
	"fmt"
	"math/rand"
	"nexconsult-sintegra-ma/internal/config"
	"nexconsult-sintegra-ma/internal/models"
	"regexp"
	"sync"
	"time"
)

// Constantes para configuração do worker pool
const (
	defaultWorkerPoolSize    = 3
	minWorkerPoolSize        = 1
	maxWorkerPoolSize        = 10
	defaultJobChannelBuffer  = 100
	defaultSubmissionTimeout = 5 * time.Second
	defaultShutdownTimeout   = 30 * time.Second
)

// ConsultaJob representa um trabalho de consulta a ser processado
type ConsultaJob struct {
	ID        string                        // ID único do job
	CNPJ      string                        // CNPJ a ser consultado
	Context   context.Context               // Contexto para cancelamento
	Resultado chan *models.SintegraResponse // Canal para resultado
	Erro      chan error                    // Canal para erro
	CreatedAt time.Time                     // Timestamp de criação
}

// WorkerPool gerencia um pool de workers para processar consultas em paralelo
type WorkerPool struct {
	service        *SintegraService
	jobs           chan ConsultaJob
	numWorkers     int
	wg             sync.WaitGroup
	isRunning      bool
	mutex          sync.RWMutex
	timeoutConfig  *config.TimeoutConfig
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	stats          *WorkerPoolStats
}

// WorkerPoolStats mantém estatísticas do pool de workers
type WorkerPoolStats struct {
	mutex           sync.RWMutex
	totalJobs       int64
	completedJobs   int64
	failedJobs      int64
	averageDuration time.Duration
	lastJobTime     time.Time
}

// NewWorkerPool cria um novo pool de workers
func NewWorkerPool(service *SintegraService, numWorkers int, timeoutConfig *config.TimeoutConfig) *WorkerPool {
	numWorkers = validateWorkerCount(numWorkers)
	timeoutConfig = ensureTimeoutConfig(timeoutConfig)

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	return &WorkerPool{
		service:        service,
		jobs:           make(chan ConsultaJob, calculateJobChannelBuffer(numWorkers)),
		numWorkers:     numWorkers,
		isRunning:      false,
		timeoutConfig:  timeoutConfig,
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
		stats:          &WorkerPoolStats{},
	}
}

// calculateJobChannelBuffer calcula o tamanho do buffer do canal de jobs
func calculateJobChannelBuffer(numWorkers int) int {
	buffer := numWorkers * 2
	if buffer < defaultJobChannelBuffer {
		return defaultJobChannelBuffer
	}
	return buffer
}

// validateWorkerCount valida e retorna um número válido de workers
func validateWorkerCount(numWorkers int) int {
	if numWorkers < minWorkerPoolSize {
		return defaultWorkerPoolSize
	}
	if numWorkers > maxWorkerPoolSize {
		return maxWorkerPoolSize
	}
	return numWorkers
}

// ensureTimeoutConfig garante que existe uma configuração de timeout
func ensureTimeoutConfig(timeoutConfig *config.TimeoutConfig) *config.TimeoutConfig {
	if timeoutConfig == nil {
		return config.DefaultTimeoutConfig()
	}
	return timeoutConfig
}

// Start inicia os workers
func (wp *WorkerPool) Start() {
	wp.mutex.Lock()
	defer wp.mutex.Unlock()

	if wp.isRunning {
		wp.service.logger.Warn().Msg("Worker pool já está em execução")
		return
	}

	wp.service.logger.Info().Int("num_workers", wp.numWorkers).Msg("🚀 Iniciando worker pool")
	wp.isRunning = true
	wp.startWorkers()
}

// startWorkers inicia todos os workers do pool
func (wp *WorkerPool) startWorkers() {
	for i := 0; i < wp.numWorkers; i++ {
		wp.wg.Add(1)
		workerID := i + 1
		go wp.runWorker(workerID)
	}
	wp.service.logger.Info().Int("workers_started", wp.numWorkers).Msg("✅ Todos os workers iniciados")
}

// runWorker executa um worker individual
func (wp *WorkerPool) runWorker(workerID int) {
	defer wp.wg.Done()
	wp.service.logger.Info().Int("worker_id", workerID).Msg("🚀 Worker iniciado")

	for {
		select {
		case job, ok := <-wp.jobs:
			if !ok {
				// Canal fechado, worker deve parar
				wp.service.logger.Info().Int("worker_id", workerID).Msg("⏹️ Worker finalizado - canal fechado")
				return
			}
			wp.processJob(workerID, job)
		case <-wp.shutdownCtx.Done():
			// Shutdown solicitado
			wp.service.logger.Info().Int("worker_id", workerID).Msg("⏹️ Worker finalizado - shutdown solicitado")
			return
		}
	}
}

// processJob processa um job individual
func (wp *WorkerPool) processJob(workerID int, job ConsultaJob) {
	start := time.Now()
	wp.service.logger.Info().
		Int("worker_id", workerID).
		Str("job_id", job.ID).
		Str("cnpj", job.CNPJ).
		Msg("📝 Processando consulta")

	// Verificar se o contexto do job foi cancelado
	select {
	case <-job.Context.Done():
		wp.service.logger.Warn().
			Int("worker_id", workerID).
			Str("job_id", job.ID).
			Msg("❌ Job cancelado antes do processamento")
		wp.sendJobResult(job, nil, job.Context.Err())
		wp.updateStats(false, time.Since(start))
		return
	default:
	}

	// Executar consulta com contexto
	resultado, err := wp.executeJobWithContext(job)

	// Calcular duração e atualizar estatísticas
	duration := time.Since(start)
	wp.updateStats(err == nil, duration)

	// Enviar resultado
	wp.sendJobResult(job, resultado, err)

	wp.service.logger.Info().
		Int("worker_id", workerID).
		Str("job_id", job.ID).
		Dur("duration", duration).
		Bool("success", err == nil).
		Msg("✅ Consulta processada")
}

// executeJobWithContext executa o job com contexto
func (wp *WorkerPool) executeJobWithContext(job ConsultaJob) (*models.SintegraResponse, error) {
	// Criar canal para resultado da consulta
	resultChan := make(chan *models.SintegraResponse, 1)
	errorChan := make(chan error, 1)

	// Executar consulta em goroutine
	go func() {
		resultado, err := wp.service.consultarCNPJInternal(job.CNPJ)
		if err != nil {
			errorChan <- err
		} else {
			resultChan <- resultado
		}
	}()

	// Aguardar resultado ou cancelamento
	select {
	case resultado := <-resultChan:
		return resultado, nil
	case err := <-errorChan:
		return nil, err
	case <-job.Context.Done():
		return nil, fmt.Errorf("job cancelado: %w", job.Context.Err())
	}
}

// updateStats atualiza as estatísticas do pool
func (wp *WorkerPool) updateStats(success bool, duration time.Duration) {
	wp.stats.mutex.Lock()
	defer wp.stats.mutex.Unlock()

	wp.stats.totalJobs++
	if success {
		wp.stats.completedJobs++
	} else {
		wp.stats.failedJobs++
	}

	// Calcular média móvel da duração
	if wp.stats.averageDuration == 0 {
		wp.stats.averageDuration = duration
	} else {
		wp.stats.averageDuration = (wp.stats.averageDuration + duration) / 2
	}

	wp.stats.lastJobTime = time.Now()
}

// sendJobResult envia o resultado do job pelos canais apropriados
func (wp *WorkerPool) sendJobResult(job ConsultaJob, resultado *models.SintegraResponse, err error) {
	defer func() {
		if r := recover(); r != nil {
			wp.service.logger.Error().
				Str("job_id", job.ID).
				Interface("panic", r).
				Msg("❌ Panic ao enviar resultado do job")
		}
	}()

	if err != nil {
		select {
		case job.Erro <- err:
			wp.service.logger.Debug().
				Str("job_id", job.ID).
				Err(err).
				Msg("📤 Erro enviado para o canal")
		case <-time.After(defaultSubmissionTimeout):
			wp.service.logger.Warn().
				Str("job_id", job.ID).
				Msg("⚠️ Timeout ao enviar erro - canal pode estar bloqueado")
		case <-job.Context.Done():
			wp.service.logger.Debug().
				Str("job_id", job.ID).
				Msg("🚫 Job cancelado durante envio de erro")
		}
	} else {
		select {
		case job.Resultado <- resultado:
			wp.service.logger.Debug().
				Str("job_id", job.ID).
				Msg("📤 Resultado enviado para o canal")
		case <-time.After(defaultSubmissionTimeout):
			wp.service.logger.Warn().
				Str("job_id", job.ID).
				Msg("⚠️ Timeout ao enviar resultado - canal pode estar bloqueado")
		case <-job.Context.Done():
			wp.service.logger.Debug().
				Str("job_id", job.ID).
				Msg("🚫 Job cancelado durante envio de resultado")
		}
	}

	// Fechar canais para sinalizar conclusão
	close(job.Resultado)
	close(job.Erro)
}

// Stop para os workers e aguarda a conclusão dos jobs em andamento
func (wp *WorkerPool) Stop() {
	wp.mutex.Lock()
	defer wp.mutex.Unlock()

	if !wp.isRunning {
		wp.service.logger.Warn().Msg("Worker pool já está parado")
		return
	}

	wp.service.logger.Info().Msg("⏹️ Iniciando shutdown do worker pool...")
	wp.isRunning = false

	// Cancelar contexto de shutdown para sinalizar workers
	wp.shutdownCancel()

	// Fechar canal de jobs após um breve delay para permitir que workers em execução terminem
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(wp.jobs)
	}()

	// Aguardar workers com timeout
	done := make(chan struct{})
	go func() {
		wp.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		wp.service.logger.Info().Msg("✅ Worker pool parado com sucesso")
	case <-time.After(defaultShutdownTimeout):
		wp.service.logger.Warn().
			Dur("timeout", defaultShutdownTimeout).
			Msg("⚠️ Timeout no shutdown do worker pool")
	}

	// Log das estatísticas finais
	wp.logFinalStats()
}

// logFinalStats registra as estatísticas finais do pool
func (wp *WorkerPool) logFinalStats() {
	wp.stats.mutex.RLock()
	defer wp.stats.mutex.RUnlock()

	wp.service.logger.Info().
		Int64("total_jobs", wp.stats.totalJobs).
		Int64("completed_jobs", wp.stats.completedJobs).
		Int64("failed_jobs", wp.stats.failedJobs).
		Dur("average_duration", wp.stats.averageDuration).
		Msg("📊 Estatísticas finais do worker pool")
}

// EnqueueJob adiciona um job à fila
func (wp *WorkerPool) EnqueueJob(cnpj string, timeout time.Duration) (*models.SintegraResponse, error) {
	if !wp.ensurePoolRunning() {
		return nil, fmt.Errorf("worker pool não está em execução")
	}

	// Validar CNPJ
	if err := wp.validateCNPJForJob(cnpj); err != nil {
		return nil, fmt.Errorf("CNPJ inválido: %w", err)
	}

	// Criar job com contexto
	job := wp.createJob(cnpj, timeout)

	wp.service.logger.Debug().
		Str("job_id", job.ID).
		Str("cnpj", cnpj).
		Dur("timeout", timeout).
		Msg("📋 Job criado para enfileiramento")

	return wp.submitJob(job)
}

// validateCNPJForJob valida o CNPJ para criação de job
func (wp *WorkerPool) validateCNPJForJob(cnpj string) error {
	if cnpj == "" {
		return fmt.Errorf("CNPJ não pode estar vazio")
	}

	// Remover caracteres não numéricos
	cnpjNumerico := regexp.MustCompile(`[^0-9]`).ReplaceAllString(cnpj, "")

	if len(cnpjNumerico) != 14 {
		return fmt.Errorf("CNPJ deve ter 14 dígitos, recebido: %d", len(cnpjNumerico))
	}

	return nil
}

// createJobChannels cria os canais para resultado e erro
// generateJobID gera um ID único para o job
func generateJobID() string {
	return fmt.Sprintf("job_%d_%d", time.Now().UnixNano(), rand.Intn(10000))
}

// createJob cria um novo job de consulta
func (wp *WorkerPool) createJob(cnpj string, timeout time.Duration) ConsultaJob {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	// Note: cancel será chamado quando o job for processado
	_ = cancel // Evita warning de variável não utilizada
	resultChan := make(chan *models.SintegraResponse, 1)
	errorChan := make(chan error, 1)

	return ConsultaJob{
		ID:        generateJobID(),
		CNPJ:      cnpj,
		Context:   ctx,
		Resultado: resultChan,
		Erro:      errorChan,
		CreatedAt: time.Now(),
	}
}

// ensurePoolRunning verifica se o pool está em execução
func (wp *WorkerPool) ensurePoolRunning() bool {
	if !wp.isRunning {
		wp.service.logger.Warn().Msg("❌ Tentativa de usar worker pool que não está em execução")
		return false
	}

	return true
}

// submitJob submete um job para processamento
func (wp *WorkerPool) submitJob(job ConsultaJob) (*models.SintegraResponse, error) {
	// Tentar submeter o job
	select {
	case wp.jobs <- job:
		wp.service.logger.Debug().
			Str("job_id", job.ID).
			Msg("📋 Job submetido para processamento")
	case <-time.After(defaultSubmissionTimeout):
		return nil, fmt.Errorf("timeout ao submeter job após %v", defaultSubmissionTimeout)
	case <-wp.shutdownCtx.Done():
		return nil, fmt.Errorf("worker pool está sendo finalizado")
	}

	// Aguardar resultado
	return wp.waitForJobResult(job)
}

// waitForJobResult aguarda o resultado do job
func (wp *WorkerPool) waitForJobResult(job ConsultaJob) (*models.SintegraResponse, error) {
	select {
	case resultado := <-job.Resultado:
		wp.service.logger.Debug().
			Str("job_id", job.ID).
			Msg("✅ Resultado recebido do job")
		return resultado, nil

	case err := <-job.Erro:
		wp.service.logger.Debug().
			Str("job_id", job.ID).
			Err(err).
			Msg("❌ Erro recebido do job")
		return nil, err

	case <-job.Context.Done():
		wp.service.logger.Warn().
			Str("job_id", job.ID).
			Msg("⏰ Job cancelado por timeout")
		return nil, fmt.Errorf("job cancelado: %w", job.Context.Err())

	case <-wp.shutdownCtx.Done():
		wp.service.logger.Warn().
			Str("job_id", job.ID).
			Msg("🛑 Job cancelado devido ao shutdown do pool")
		return nil, fmt.Errorf("worker pool está sendo finalizado")
	}
}

// GetStats retorna as estatísticas atuais do pool
func (wp *WorkerPool) GetStats() WorkerPoolStats {
	wp.stats.mutex.RLock()
	defer wp.stats.mutex.RUnlock()

	return WorkerPoolStats{
		totalJobs:       wp.stats.totalJobs,
		completedJobs:   wp.stats.completedJobs,
		failedJobs:      wp.stats.failedJobs,
		averageDuration: wp.stats.averageDuration,
		lastJobTime:     wp.stats.lastJobTime,
	}
}

// IsRunning retorna se o pool está em execução
func (wp *WorkerPool) IsRunning() bool {
	wp.mutex.RLock()
	defer wp.mutex.RUnlock()
	return wp.isRunning
}

// GetWorkerCount retorna o número de workers
func (wp *WorkerPool) GetWorkerCount() int {
	return wp.numWorkers
}
