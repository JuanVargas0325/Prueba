# Paso 52 · IA: proveedor compatible con OpenAI

> Rama `paso-a-paso` · paso 52 de 65 · anterior: **Paso 51 · Interfaz de IA, modo simulado y prompt** · siguiente: **Paso 53 · IA: proveedor Anthropic (Claude)**

---

## 1. Qué se hizo en este paso

Se agregó un proveedor real de IA que habla el protocolo **Chat Completions de OpenAI**, el formato que hoy entienden casi todos los servicios de modelos.

| Archivo | Cambio |
|---|---|
| `backend/internal/ia/openai.go` | **Nuevo.** `OpenAI` con `NuevoOpenAI` y `Resumir`: arma la petición HTTP, la envía y clasifica los errores. |
| `backend/internal/ia/ia.go` | **Modificado.** Constante `ProveedorOpenAI`, el `case` en `Nuevo` y las auxiliares `estadoReintentable` y `limpiarResumen`. |
| `backend/internal/ia/ia_test.go` | **Modificado.** Tres pruebas con un **servidor HTTP falso** y la validación de `openai` sin modelo. |
| `.env.example` | **Modificado.** Explicación de `openai` y ejemplos para Gemini, OpenAI y Groq. |

---

## 2. Por qué

- **Un solo cliente, muchos proveedores.** OpenAI, **Gemini** (Google AI Studio), **Groq**, **OpenRouter**, **Ollama** (modelo local, paso 57) y LM Studio exponen la misma API `/chat/completions`. Con este cliente el equipo puede elegir cualquiera cambiando `LLM_URL_BASE`, `LLM_API_KEY` y `LLM_MODELO`.
- **Sin dependencias extra:** se usa `net/http` de la librería estándar; no hace falta el SDK de cada proveedor.
- **Errores clasificados:** distinguir "reintentar más tarde" (límite de uso, caída) de "no va a funcionar" (clave inválida) evita gastar reintentos inútiles.

---

## 3. Explicación detallada

### 3.1 Construcción y validación

```go
const URLBaseOpenAIPorDefecto = "https://api.openai.com/v1"

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
```
- El **modelo es obligatorio**: cada servicio tiene los suyos y no hay uno por defecto que sirva para todos.
- `TrimRight(..., "/")` evita una URL con doble barra si `LLM_URL_BASE` termina en `/`.
- El `http.Client` tiene un **tiempo máximo** (`LLM_TIMEOUT`, 2 min por defecto): una llamada colgada no bloquea el worker.
- El cliente HTTP se recibe como parámetro: las pruebas pasan `nil` y apuntan la URL a un servidor falso.

### 3.2 La petición

```go
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
```

```go
	cuerpo, err := json.Marshal(peticionChat{
		Model: o.cfg.Modelo,
		Messages: []mensajeChat{
			{Role: "system", Content: InstruccionesSistema},
			{Role: "user", Content: ConstruirMensajeUsuario(datos)},
		},
		MaxTokens:   o.cfg.MaxTokens,
		Temperature: o.cfg.Temperatura,
	})
	...
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.url, bytes.NewReader(cuerpo))
	req.Header.Set("Content-Type", "application/json")
	if o.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.cfg.APIKey)
	}
	resp, err := o.client.Do(req)
```
- Dos mensajes: el de **sistema** (las reglas, paso 51) y el de **usuario** (los datos de la semana).
- `omitempty` en `max_tokens` y `temperature`: si no se configuran, **no se envían** y el proveedor usa sus valores por defecto (algunos modelos rechazan ciertos valores).
- La clave va en la cabecera `Authorization: Bearer`. Si está vacía no se envía (Ollama local no la necesita).
- `NewRequestWithContext(ctx, ...)`: si la cola cancela el trabajo por tiempo agotado (paso 47), la llamada HTTP se aborta.

### 3.3 La respuesta y los errores

```go
	defer func() { _ = resp.Body.Close() }()
	contenido, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	...
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
```
- La respuesta se lee con un **límite de 4 MB** (`4<<20`): un servidor que respondiera algo gigante no agota la memoria.
- Se cierra el cuerpo con `defer` (obligatorio en Go para liberar la conexión).
- Si el código no es 200, se arma un `ErrorProveedor` con el mensaje del proveedor (recortado a 500 caracteres) para guardarlo en el reporte fallido.
- Un error de **red** (no se pudo conectar) se devuelve envuelto con `%w`; como no es `ErrorProveedor`, se considera reintentable (paso 51).

```go
func estadoReintentable(estado int) bool {
	return estado == http.StatusRequestTimeout ||
		estado == http.StatusConflict ||
		estado == http.StatusTooManyRequests ||
		estado >= http.StatusInternalServerError
}
```

| Código | Significado | ¿Reintentar? |
|---|---|---|
| 400 | Petición inválida (por ejemplo, modelo inexistente) | No |
| 401 / 403 | Clave inválida o sin permiso | No |
| 408 / 409 | Tiempo agotado o conflicto temporal | Sí |
| 429 | Límite de uso alcanzado | Sí (más tarde) |
| 5xx | Falla del servidor del proveedor | Sí |

```go
func limpiarResumen(proveedor, texto string) (string, error) {
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return "", &ErrorProveedor{Proveedor: proveedor, Mensaje: "el modelo devolvió un resumen vacío", Reintentable: true}
	}
	return texto, nil
}
```
- Un resumen vacío no sirve para el PDF: se trata como fallo transitorio (otra llamada puede funcionar).

---

## 4. Las pruebas: un servidor HTTP falso

```go
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		... verifica la ruta y la cabecera, guarda la petición ...
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"  Resumen de prueba.  "}}]}`))
	}))
	defer srv.Close()
	o, err := NuevoOpenAI(Config{URLBase: srv.URL + "/v1/", APIKey: "clave-prueba", Modelo: "modelo-x", ...}, nil)
```
- `httptest.NewServer` levanta un servidor **real** en un puerto local, solo durante la prueba. Así se prueba el cliente HTTP completo (serialización, cabeceras, lectura) **sin internet y sin gastar dinero**.

| Prueba | Qué verifica |
|---|---|
| `TestOpenAIEnviaPeticionYLeeRespuesta` | Ruta `/v1/chat/completions` (con la barra final de la URL bien manejada), cabecera `Bearer`, modelo, dos mensajes (`system` primero), resumen sin espacios sobrantes y que `temperature` y `max_tokens` **no** se envían si no se configuran. |
| `TestOpenAIClasificaErrores` | 401 y 400 → no reintentables; 429 y 503 → reintentables; el mensaje del proveedor se conserva. |
| `TestOpenAIRespuestaVaciaEsReintentable` | Un contenido vacío es un error reintentable. |
| `TestNuevoValidaConfiguracion` | `openai` sin modelo es un error; `OpenAI` (mayúsculas) con modelo se crea. |

---

## 5. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| `net/http` como cliente | `http.NewRequestWithContext` + `client.Do(req)`. |
| Cerrar el cuerpo | `defer resp.Body.Close()` libera la conexión. |
| `httptest.NewServer` | Servidor HTTP temporal para probar clientes. |
| `omitempty` en peticiones | No enviar campos no configurados. |
| API compatible | Un mismo protocolo implementado por varios proveedores. |

---

## 6. Cómo comprobarlo

```bash
cd backend
go test ./internal/ia/ -v
```

Con una cuenta real (por ejemplo, Gemini de Google AI Studio, que tiene capa gratuita), en su `.env`:

```dotenv
LLM_PROVEEDOR=openai
LLM_URL_BASE=https://generativelanguage.googleapis.com/v1beta/openai
LLM_API_KEY=<su clave>
LLM_MODELO=<un modelo Gemini disponible en su cuenta>
```

El worker que usará esta configuración llega en el paso 55.

---

## 7. Pasos para lograr esta versión desde el paso 51

1. Crear `backend/internal/ia/openai.go` (`URLBaseOpenAIPorDefecto`, `OpenAI`, `NuevoOpenAI`, `Proveedor`, `Modelo`, los tipos `mensajeChat`, `peticionChat`, `respuestaChat` y `Resumir`).
2. En `ia.go`: constante `ProveedorOpenAI`, `case ProveedorOpenAI` en `Nuevo` (con `&http.Client{Timeout: cfg.Timeout}`), `estadoReintentable` y `limpiarResumen`.
3. En `ia_test.go`: las tres pruebas de OpenAI y las dos verificaciones nuevas en `TestNuevoValidaConfiguracion`.
4. En `.env.example`: la línea de `openai` y los ejemplos de Gemini, OpenAI y Groq.
5. `make pruebas lint`
6. Commit: `git add . && git commit -m "Paso 52: proveedor de IA compatible con OpenAI"`

---

## 8. Preparación para las próximas entregas

- **Vertex AI (GCP)** también ofrece un punto de acceso compatible con OpenAI para los modelos Gemini. Si el curso pide usar servicios del proveedor de nube para la IA, bastará con cambiar `LLM_URL_BASE` y la clave.

---

## 9. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-14 Resumen con IA | Proveedor real configurable por variables. |
| Stack: LLM en la nube o local | El mismo cliente sirve para servicios en la nube y para Ollama. |
| RNF-01 Backend sólido | Tiempo máximo, límite de respuesta y errores clasificados. |
