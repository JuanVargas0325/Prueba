package ia

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"seguimiento/internal/dominio"
)

const (
	// ModeloAnthropicPorDefecto se usa cuando LLM_MODELO está vacío.
	ModeloAnthropicPorDefecto = "claude-opus-5-5"
	maxTokensAnthropic        = 16000
)

// Anthropic usa el SDK oficial de Anthropic para llamar a Claude.
type Anthropic struct {
	client    anthropic.Client
	modelo    string
	maxTokens int64
}

// NuevoAnthropic valida la configuración y crea el cliente.
func NuevoAnthropic(cfg Config) (*Anthropic, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("LLM_API_KEY es obligatorio con el proveedor anthropic")
	}
	opciones := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
		option.WithRequestTimeout(cfg.Timeout),
		// Los reintentos los gestiona la cola de tareas (Asynq).
		option.WithMaxRetries(0),
	}
	if cfg.URLBase != "" {
		opciones = append(opciones, option.WithBaseURL(cfg.URLBase))
	}
	modelo := cfg.Modelo
	if modelo == "" {
		modelo = ModeloAnthropicPorDefecto
	}
	maxTokens := int64(cfg.MaxTokens)
	if maxTokens <= 0 {
		maxTokens = maxTokensAnthropic
	}
	return &Anthropic{client: anthropic.NewClient(opciones...), modelo: modelo, maxTokens: maxTokens}, nil
}

func (a *Anthropic) Proveedor() string { return ProveedorAnthropic }
func (a *Anthropic) Modelo() string    { return a.modelo }

// Resumir envía los datos de la semana a Claude y devuelve el resumen.
func (a *Anthropic) Resumir(ctx context.Context, datos dominio.DatosSemanaPersona) (string, error) {
	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     a.modelo,
		MaxTokens: a.maxTokens,
		System:    []anthropic.TextBlockParam{{Text: InstruccionesSistema}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(ConstruirMensajeUsuario(datos))),
		},
	})
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			return "", &ErrorProveedor{
				Proveedor:    ProveedorAnthropic,
				Estado:       apiErr.StatusCode,
				Mensaje:      apiErr.Error(),
				Reintentable: estadoReintentable(apiErr.StatusCode),
			}
		}
		return "", fmt.Errorf("llamando a Anthropic: %w", err)
	}

	if resp.StopReason == anthropic.StopReasonRefusal {
		return "", &ErrorProveedor{Proveedor: ProveedorAnthropic, Mensaje: "el modelo rechazó la solicitud"}
	}

	var texto strings.Builder
	for _, bloque := range resp.Content {
		if t, ok := bloque.AsAny().(anthropic.TextBlock); ok {
			texto.WriteString(t.Text)
		}
	}
	return limpiarResumen(ProveedorAnthropic, texto.String())
}
