package websocket

import (
	"time"

	"nexconsult/internal/types"
)

// MessageType tipos de mensagens WebSocket
type MessageType string

const (
	// Mensagens do cliente para servidor
	MessageTypeConsultaCNPJ MessageType = "consulta_cnpj"
	MessageTypePing         MessageType = "ping"
	
	// Mensagens do servidor para cliente
	MessageTypeStatus       MessageType = "status"
	MessageTypeProgress     MessageType = "progress"
	MessageTypeResult       MessageType = "result"
	MessageTypeError        MessageType = "error"
	MessageTypePong         MessageType = "pong"
	MessageTypeQueueStatus  MessageType = "queue_status"
)

// WebSocketMessage estrutura base para mensagens
type WebSocketMessage struct {
	Type      MessageType `json:"type"`
	ID        string      `json:"id,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data,omitempty"`
}

// ConsultaCNPJRequest requisição de consulta CNPJ
type ConsultaCNPJRequest struct {
	CNPJ string `json:"cnpj"`
}

// StatusMessage mensagem de status
type StatusMessage struct {
	Status      string `json:"status"`
	Message     string `json:"message"`
	CNPJ        string `json:"cnpj,omitempty"`
	JobID       string `json:"job_id,omitempty"`
	QueueSize   int    `json:"queue_size,omitempty"`
	Position    int    `json:"position,omitempty"`
	EstimatedTime string `json:"estimated_time,omitempty"`
}

// ProgressMessage mensagem de progresso
type ProgressMessage struct {
	CNPJ        string `json:"cnpj"`
	JobID       string `json:"job_id"`
	Stage       string `json:"stage"`
	Progress    int    `json:"progress"`    // 0-100
	Message     string `json:"message"`
	ElapsedTime string `json:"elapsed_time"`
}

// ResultMessage mensagem de resultado
type ResultMessage struct {
	CNPJ     string           `json:"cnpj"`
	JobID    string           `json:"job_id"`
	Success  bool             `json:"success"`
	Data     *types.CNPJData  `json:"data,omitempty"`
	Error    string           `json:"error,omitempty"`
	Duration string           `json:"duration"`
}

// ErrorMessage mensagem de erro
type ErrorMessage struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// QueueStatusMessage status da fila
type QueueStatusMessage struct {
	TotalJobs     int64 `json:"total_jobs"`
	QueueSize     int   `json:"queue_size"`
	ActiveWorkers int   `json:"active_workers"`
	ProcessingJobs []JobInfo `json:"processing_jobs"`
}

// JobInfo informações de um job
type JobInfo struct {
	ID       string    `json:"id"`
	CNPJ     string    `json:"cnpj"`
	Status   string    `json:"status"`
	Started  time.Time `json:"started"`
	Progress int       `json:"progress"`
}

// Client representa um cliente WebSocket conectado
type Client struct {
	ID       string
	Conn     interface{} // Será definido como *websocket.Conn
	Send     chan WebSocketMessage
	Jobs     map[string]*JobInfo // Jobs ativos deste cliente
	LastPing time.Time
}

// Hub gerencia clientes WebSocket
type Hub struct {
	Clients    map[string]*Client
	Register   chan *Client
	Unregister chan *Client
	Broadcast  chan WebSocketMessage
	JobUpdates chan JobUpdate
}

// JobUpdate atualização de job
type JobUpdate struct {
	ClientID string
	JobID    string
	Type     MessageType
	Data     interface{}
}
