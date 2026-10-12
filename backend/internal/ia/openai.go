package ia

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"seguimiento/internal/dominio"
)

// URLBaseOpenAIPorDefecto se usa cuando LLM_URL_BASE está vacío.
const URLBaseOpenAIPorDefecto = "https://api.openai.com/v1"

// OpenAI llama al endpoint /chat/completions de OpenAI o de cualquier
// servicio que implemente la misma API. Ejemplos de LLM_URL_BASE:
//
//	Gemini:     https://generativelanguage.googleapis.com/v1beta/openai
//	Groq:       https://api.groq.com/openai/v1
//	OpenRouter: https://openrouter.ai/api/v1
//	Ollama:     http://ollama:11434/v1
type OpenAI struct {
	cfg    Config
	url    string
	client *http.Client
}

// NuevoOpenAI valida la configuración y crea el cliente.
func NuevoOpenAI(cfg Config, client *http.Client) (*OpenAI, error) {
	if cfg.Modelo == "" {
		return nil, errors.New("LLM_MODELO es obligatorio con el proveedor openai")
	}
	base := strings.TrimRight(cfg.URLBase, "/")
	if base == "" {
		base = URLBaseOpenAIPorDefecto
	}
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	}
	return &OpenAI{cfg: cfg, url: base + "/chat/completions", client: client}, nil
}

func (o *OpenAI) Proveedor() string { return ProveedorOpenAI }
func (o *OpenAI) Modelo() string    { return o.cfg.Modelo }

type mensajeChat struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type peticionChat struct {
	Model       string        `json:"model"`
	Messages    []mensajeChat `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
}

type respuestaChat struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Resumir envía los datos de la semana al modelo y devuelve el resumen.
func (o *OpenAI) Resumir(ctx context.Context, datos dominio.DatosSemanaPersona) (string, error) {
	cuerpo, err := json.Marshal(peticionChat{
		Model: o.cfg.Modelo,
		Messages: []mensajeChat{
			{Role: "system", Content: InstruccionesSistema},
			{Role: "user", Content: ConstruirMensajeUsuario(datos)},
		},
		MaxTokens:   o.cfg.MaxTokens,
		Temperature: o.cfg.Temperatura,
	})
	if err != nil {
		return "", fmt.Errorf("serializando petición: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.url, bytes.NewReader(cuerpo))
	if err != nil {
		return "", &ErrorProveedor{Proveedor: ProveedorOpenAI, Mensaje: "URL inválida: " + err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	if o.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.cfg.APIKey)
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("llamando al proveedor de IA: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	contenido, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return "", fmt.Errorf("leyendo respuesta del proveedor de IA: %w", err)
	}

	var r respuestaChat
	_ = json.Unmarshal(contenido, &r)

	if resp.StatusCode != http.StatusOK {
		mensaje := strings.TrimSpace(string(contenido))
		if r.Error != nil && r.Error.Message != "" {
			mensaje = r.Error.Message
		}
		if len(mensaje) > 500 {
			mensaje = mensaje[:500]
		}
		return "", &ErrorProveedor{
			Proveedor:    ProveedorOpenAI,
			Estado:       resp.StatusCode,
			Mensaje:      mensaje,
			Reintentable: estadoReintentable(resp.StatusCode),
		}
	}
	if len(r.Choices) == 0 {
		return "", &ErrorProveedor{Proveedor: ProveedorOpenAI, Mensaje: "respuesta sin opciones", Reintentable: true}
	}
	return limpiarResumen(ProveedorOpenAI, r.Choices[0].Message.Content)
}
