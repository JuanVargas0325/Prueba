# Paso 53 · IA: proveedor Anthropic (Claude)

> Rama `paso-a-paso` · paso 53 de 65 · anterior: **Paso 52 · Proveedor compatible con OpenAI** · siguiente: **Paso 54 · Generación asíncrona de reportes**

---

## 1. Qué se hizo en este paso

Se agregó un tercer proveedor de IA: **Claude**, de Anthropic, usando su **SDK oficial para Go**.

| Archivo | Cambio |
|---|---|
| `backend/go.mod`, `go.sum` | Nueva dependencia `github.com/anthropics/anthropic-sdk-go v1.80.0`. Con ella, `go.mod` y `go.sum` quedan **idénticos a los de la versión final**. |
| `backend/internal/ia/anthropic.go` | **Nuevo.** `Anthropic` con `NuevoAnthropic` y `Resumir`. |
| `backend/internal/ia/ia.go` | **Modificado.** Constante `ProveedorAnthropic` y su `case` en `Nuevo`. |
| `backend/internal/ia/ia_test.go` | **Modificado.** Dos pruebas con un servidor falso que imita la API de Anthropic y la validación de la clave obligatoria. |
| `.env.example` | **Modificado.** Opción `anthropic` y su ejemplo. |

---

## 2. Por qué

- La API de Anthropic **no** es compatible con el formato de OpenAI (usa `/v1/messages`, otra autenticación y otra forma de respuesta), así que necesita su propio cliente.
- **SDK oficial en lugar de HTTP a mano:** el SDK mantiene al día los tipos de la API, maneja los detalles del protocolo y expone errores tipados (`*anthropic.Error` con el código HTTP). Es la forma recomendada por el proveedor.
- Da al equipo otra opción de modelo en la nube, intercambiable por variables de entorno como las demás.

---

## 3. Explicación detallada de `anthropic.go`

### 3.1 Construcción

```go
const (
	// ModeloAnthropicPorDefecto se usa cuando LLM_MODELO está vacío.
	ModeloAnthropicPorDefecto = "claude-opus-5-5"
	maxTokensAnthropic        = 16000
)

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
```
- **La clave es obligatoria**; el modelo no (por defecto, `claude-opus-5-5`).
- **`WithMaxRetries(0)`:** el SDK reintenta por su cuenta si no se le indica lo contrario. Aquí los reintentos ya los hace la cola (paso 47) con su propia espera; si ambos reintentaran, una falla generaría hasta 3 × 3 llamadas. **Una sola capa decide los reintentos.**
- `WithBaseURL` permite apuntar a otra URL: lo usan las pruebas (servidor falso) y serviría para un proxy corporativo.
- `max_tokens` es **obligatorio** en la API de Anthropic. Se usa 16 000 por defecto: el resumen es corto (máximo 250 palabras según las instrucciones), pero un límite holgado evita que la respuesta se corte.
- `option.RequestOption` es otro ejemplo de **opciones funcionales** (como las de JWT en el paso 12).

### 3.2 `Resumir`

```go
func (a *Anthropic) Resumir(ctx context.Context, datos dominio.DatosSemanaPersona) (string, error) {
	resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     a.modelo,
		MaxTokens: a.maxTokens,
		System:    []anthropic.TextBlockParam{{Text: InstruccionesSistema}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(ConstruirMensajeUsuario(datos))),
		},
	})
```
- El **mismo prompt** del paso 51: las instrucciones van en `System` y los datos en el mensaje del usuario. Las reglas de privacidad y de "no evaluar" son idénticas para todos los proveedores.

```go
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
```
- El SDK devuelve un `*anthropic.Error` con el código HTTP; se convierte al `ErrorProveedor` común con la **misma clasificación** del paso 52 (401 no reintentable, 429/5xx sí). Los errores de red se envuelven y son reintentables.

```go
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
```
- Si el modelo **se niega** a responder (`StopReasonRefusal`), es un error **no reintentable** (`Reintentable` queda en su valor cero, `false`): repetir la misma petición daría lo mismo.
- La respuesta llega como una lista de **bloques de contenido** de distintos tipos. Se juntan solo los de **texto**, con una **aserción de tipo** (paso 16) sobre `bloque.AsAny()`.

---

## 4. Las pruebas

El servidor falso (`httptest`) responde exactamente como la API de Anthropic:

```go
	_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5-5",
		"content":[{"type":"text","text":"Resumen de Claude."}],
		"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`))
```

| Prueba | Qué verifica |
|---|---|
| `TestAnthropicEnviaPeticionYLeeRespuesta` | Ruta `/v1/messages`, cabecera `X-Api-Key`, modelo por defecto, que se envían las instrucciones de sistema y que el texto de la respuesta es el resumen. |
| `TestAnthropicErrorDeAutenticacionNoEsReintentable` | Un 401 (`authentication_error`) es un error de IA **no reintentable**. |
| `TestNuevoValidaConfiguracion` | `anthropic` sin clave es un error de configuración. |

---

## 5. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| SDK oficial | Librería del proveedor que encapsula su API. |
| Una sola capa de reintentos | Desactivar los del SDK porque ya los maneja la cola. |
| Bloques de contenido | La respuesta de Claude es una lista de bloques tipados; se toman los de texto. |
| Rechazo del modelo | Resultado válido de la API que se trata como error permanente. |

---

## 6. Cómo comprobarlo

```bash
cd backend
go test ./internal/ia/ -v
```

Con una clave real, en su `.env`:

```dotenv
LLM_PROVEEDOR=anthropic
LLM_API_KEY=<su clave de console.anthropic.com>
# LLM_MODELO=claude-opus-5-5   (opcional)
```

---

## 7. Pasos para lograr esta versión desde el paso 52

1. `go get github.com/anthropics/anthropic-sdk-go@v1.80.0 && go mod tidy` (desde `backend/`).
2. Crear `backend/internal/ia/anthropic.go` (`ModeloAnthropicPorDefecto`, `maxTokensAnthropic`, `Anthropic`, `NuevoAnthropic`, `Proveedor`, `Modelo`, `Resumir`).
3. En `ia.go`: constante `ProveedorAnthropic` y `case ProveedorAnthropic: return NuevoAnthropic(cfg)`, y el mensaje de error con los tres proveedores.
4. En `ia_test.go`: las dos pruebas de Anthropic y la validación de la clave.
5. En `.env.example`: la línea de `anthropic` y su ejemplo.
6. `make pruebas lint`
7. Commit: `git add . && git commit -m "Paso 53: proveedor de IA Anthropic"`

---

## 8. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-14 Resumen con IA | Tercer proveedor real, intercambiable por variables. |
| RN-43 / RN-46 | Mismo prompt (insumos permitidos, sin evaluación). |
| RNF-01 Backend sólido | Tiempo máximo, errores clasificados y reintentos solo en la cola. |
