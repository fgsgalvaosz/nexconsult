package worker

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"nexconsult/internal/browser"
	"nexconsult/internal/captcha"
	"nexconsult/internal/logger"
	"nexconsult/internal/types"
)

// WorkerPool gerencia workers para processamento de CNPJs
type WorkerPool struct {
	workers       []*Worker
	jobQueue      chan *types.Job
	resultQueue   chan types.CNPJResult
	captchaClient *captcha.SolveCaptchaClient
	browserMgr    *browser.BrowserManager

	// Estatísticas
	stats WorkerPoolStats

	// Controle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex
}

// WorkerPoolStats estatísticas do pool
type WorkerPoolStats struct {
	TotalJobs     int64     `json:"total_jobs"`
	CompletedJobs int64     `json:"completed_jobs"`
	FailedJobs    int64     `json:"failed_jobs"`
	ActiveWorkers int32     `json:"active_workers"`
	QueueSize     int       `json:"queue_size"`
	StartTime     time.Time `json:"start_time"`
}

// Worker representa um worker individual
type Worker struct {
	ID            int
	pool          *WorkerPool
	extractor     *browser.CNPJExtractor
	isActive      int32
	jobsProcessed int64
}

// NewWorkerPool cria um novo pool de workers
func NewWorkerPool(workerCount int, captchaClient *captcha.SolveCaptchaClient) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())

	// Cria browser manager otimizado para busca direta
	browserMgr := browser.NewBrowserManager(workerCount, false) // headless = false para visualizar

	pool := &WorkerPool{
		workers:       make([]*Worker, workerCount),
		jobQueue:      make(chan *types.Job, 1000), // Buffer para 1000 jobs
		resultQueue:   make(chan types.CNPJResult, 1000),
		captchaClient: captchaClient,
		browserMgr:    browserMgr,
		ctx:           ctx,
		cancel:        cancel,
		stats: WorkerPoolStats{
			StartTime: time.Now(),
		},
	}

	// Cria workers
	for i := 0; i < workerCount; i++ {
		extractor := browser.NewCNPJExtractor(captchaClient, browserMgr)
		worker := &Worker{
			ID:        i,
			pool:      pool,
			extractor: extractor,
		}
		pool.workers[i] = worker
	}

	return pool
}

// GetJobQueue retorna o canal de jobs
func (p *WorkerPool) GetJobQueue() chan<- *types.Job {
	return p.jobQueue
}

// Start inicia o pool de workers
func (wp *WorkerPool) Start() error {
	// Inicia browser manager
	if err := wp.browserMgr.Start(); err != nil {
		return fmt.Errorf("failed to start browser manager: %v", err)
	}

	// Inicia workers
	for _, worker := range wp.workers {
		wp.wg.Add(1)
		go worker.start()
	}

	logger.GetGlobalLogger().WithComponent("worker").InfoFields("Worker pool started", logger.Fields{"workers": len(wp.workers)})
	return nil
}

// Stop para o pool de workers
func (wp *WorkerPool) Stop() {
	logger.GetGlobalLogger().WithComponent("worker").Info("Stopping worker pool...")

	wp.cancel()
	close(wp.jobQueue)

	wp.wg.Wait()
	wp.browserMgr.Stop()

	logger.GetGlobalLogger().WithComponent("worker").Info("Worker pool stopped")
}

// ProcessSingle processa um único CNPJ
func (wp *WorkerPool) ProcessSingle(cnpj string, useCache bool) types.CNPJResult {
	job := &types.Job{
		ID:       uuid.New().String(),
		CNPJ:     cnpj,
		UseCache: useCache,
		Created:  time.Now(),
		Result:   make(chan types.CNPJResult, 1),
	}

	// Envia job
	select {
	case wp.jobQueue <- job:
		atomic.AddInt64(&wp.stats.TotalJobs, 1)
	case <-time.After(30 * time.Second):
		return types.CNPJResult{
			CNPJ:   cnpj,
			Error:  "timeout: queue is full",
			Status: "error",
		}
	}

	// Aguarda resultado
	select {
	case result := <-job.Result:
		return result
	case <-time.After(5 * time.Minute):
		return types.CNPJResult{
			CNPJ:   cnpj,
			Error:  "timeout: processing took too long",
			Status: "error",
		}
	}
}

// ProcessBatch processa múltiplos CNPJs
func (wp *WorkerPool) ProcessBatch(cnpjs []string, useCache bool) types.BatchResponse {
	start := time.Now()

	if len(cnpjs) == 0 {
		return types.BatchResponse{
			Results: []types.CNPJResult{},
			Stats: types.BatchStats{
				Total:     0,
				Success:   0,
				Errors:    0,
				Cached:    0,
				Duration:  0,
				StartTime: start,
				EndTime:   time.Now(),
			},
		}
	}

	// Cria jobs
	jobs := make([]*types.Job, len(cnpjs))
	for i, cnpj := range cnpjs {
		jobs[i] = &types.Job{
			ID:       uuid.New().String(),
			CNPJ:     cnpj,
			UseCache: useCache,
			Created:  time.Now(),
			Result:   make(chan types.CNPJResult, 1),
		}
	}

	// Envia todos os jobs
	for _, job := range jobs {
		select {
		case wp.jobQueue <- job:
			atomic.AddInt64(&wp.stats.TotalJobs, 1)
		case <-time.After(30 * time.Second):
			// Se não conseguir enviar, retorna erro para este CNPJ
			job.Result <- types.CNPJResult{
				CNPJ:   job.CNPJ,
				Error:  "timeout: queue is full",
				Status: "error",
			}
		}
	}

	// Coleta resultados
	results := make([]types.CNPJResult, len(jobs))
	var success, errors, cached int

	for i, job := range jobs {
		select {
		case result := <-job.Result:
			results[i] = result
			switch result.Status {
			case "success":
				success++
			case "cached":
				cached++
				success++
			case "error":
				errors++
			}
		case <-time.After(5 * time.Minute):
			results[i] = types.CNPJResult{
				CNPJ:   job.CNPJ,
				Error:  "timeout: processing took too long",
				Status: "error",
			}
			errors++
		}
	}

	end := time.Now()

	return types.BatchResponse{
		Results: results,
		Stats: types.BatchStats{
			Total:     len(cnpjs),
			Success:   success,
			Errors:    errors,
			Cached:    cached,
			Duration:  end.Sub(start),
			StartTime: start,
			EndTime:   end,
		},
	}
}

// GetStats retorna estatísticas do pool
func (wp *WorkerPool) GetStats() types.WorkerStats {
	wp.mu.RLock()
	defer wp.mu.RUnlock()

	activeWorkers := atomic.LoadInt32(&wp.stats.ActiveWorkers)

	return types.WorkerStats{
		Workers: types.WorkerInfo{
			Active: int(activeWorkers),
			Idle:   len(wp.workers) - int(activeWorkers),
			Total:  len(wp.workers),
		},
		Queue: types.QueueInfo{
			Pending:    len(wp.jobQueue),
			Processing: int(activeWorkers),
			Completed:  int(atomic.LoadInt64(&wp.stats.CompletedJobs)),
		},
		Cache: types.CacheInfo{
			HitRate: 0.0, // Cache desabilitado - sempre busca direta
			Size:    0,
			Hits:    0,
			Misses:  0,
		},
		System: types.SystemInfo{
			Uptime:    time.Since(wp.stats.StartTime).String(),
			Version:   "1.0.0",
			GoVersion: "1.21",
		},
	}
}

// start inicia o worker
func (w *Worker) start() {
	defer w.pool.wg.Done()

	logger.GetGlobalLogger().WithComponent("worker").DebugFields("Worker started", logger.Fields{"worker_id": w.ID})

	for {
		select {
		case job, ok := <-w.pool.jobQueue:
			if !ok {
				logger.GetGlobalLogger().WithComponent("worker").DebugFields("Worker stopped", logger.Fields{"worker_id": w.ID})
				return
			}

			w.processJob(job)

		case <-w.pool.ctx.Done():
			logger.GetGlobalLogger().WithComponent("worker").DebugFields("Worker stopped by context", logger.Fields{"worker_id": w.ID})
			return
		}
	}
}

// processJob processa um job
func (w *Worker) processJob(job *types.Job) {
	atomic.StoreInt32(&w.isActive, 1)
	atomic.AddInt32(&w.pool.stats.ActiveWorkers, 1)
	defer func() {
		atomic.StoreInt32(&w.isActive, 0)
		atomic.AddInt32(&w.pool.stats.ActiveWorkers, -1)
		atomic.AddInt64(&w.jobsProcessed, 1)
	}()

	job.Started = time.Now()

	logger.GetGlobalLogger().WithComponent("worker").DebugFields("Processing job", logger.Fields{
		"worker_id": w.ID,
		"job_id":    job.ID,
		"cnpj":      job.CNPJ,
	})

	// Sempre extrai diretamente do site da Receita Federal
	data, err := w.extractor.ExtractCNPJData(job.CNPJ)

	job.Finished = time.Now()

	var result types.CNPJResult
	if err != nil {
		result = types.CNPJResult{
			CNPJ:   job.CNPJ,
			Error:  err.Error(),
			Status: "error",
		}
		atomic.AddInt64(&w.pool.stats.FailedJobs, 1)

		logger.GetGlobalLogger().WithComponent("worker").ErrorFields("Job failed", logger.Fields{
			"worker_id": w.ID,
			"job_id":    job.ID,
			"cnpj":      job.CNPJ,
			"error":     err.Error(),
			"duration":  job.Finished.Sub(job.Started),
		})
	} else {
		result = types.CNPJResult{
			CNPJ:   job.CNPJ,
			Data:   data,
			Status: "success", // Sempre "success" pois sempre busca diretamente
		}
		atomic.AddInt64(&w.pool.stats.CompletedJobs, 1)

		logger.GetGlobalLogger().WithComponent("worker").InfoFields("Job completed successfully", logger.Fields{
			"worker_id": w.ID,
			"job_id":    job.ID,
			"cnpj":      job.CNPJ,
			"duration":  job.Finished.Sub(job.Started),
		})
	}

	// Envia resultado
	select {
	case job.Result <- result:
	case <-time.After(5 * time.Second):
		logger.GetGlobalLogger().WithComponent("worker").WarnFields("Failed to send result - timeout", logger.Fields{
			"worker_id": w.ID,
			"job_id":    job.ID,
		})
	}
}

// Cache removido - sempre busca diretamente no site da Receita Federal
