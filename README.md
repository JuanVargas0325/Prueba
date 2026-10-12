# Paso 50 · Generador de PDF

> Rama `paso-a-paso` · paso 50 de 65 · anterior: **Paso 49 · Endpoints de reportes** · siguiente: **Paso 51 · IA: interfaz, modo simulado y prompt**

---

## 1. Qué se hizo en este paso

Se creó el paquete que convierte los datos de la semana y el resumen en un **documento PDF**.

| Archivo | Cambio |
|---|---|
| `backend/go.mod`, `go.sum` | Nueva dependencia `codeberg.org/go-pdf/fpdf v0.12.0`. |
| `backend/internal/pdf/generador.go` | **Nuevo.** `Contenido`, `Generador` y `Generar`, con encabezado, datos generales, resumen, tabla de dedicación y detalle de tareas. |
| `backend/internal/pdf/generador_test.go` | **Nuevo.** Genera un PDF con y sin tareas y verifica que sea válido. |

**Contenido del reporte** (tamaño carta):

```
Reporte semanal de actividades
Semana del 05/10/2026 al 11/10/2026
───────────────────────────────────────────────
Persona:               José Muñoz
Correo:                jose@uniandes.edu.co
Profesor responsable:  Profesora Peña
Tareas registradas:    2
Horas reportadas:      5
Generado:              14/10/2026 10:00 -05

Resumen de la semana
  <texto generado por la IA>
  Resumen generado automáticamente ... (IA: proveedor / modelo). Sintetiza el
  trabajo reportado y no constituye una evaluación de desempeño.

Dedicación por curso o proyecto
  ┌───────────────────────────┬──────────┬─────────────┬────────────┐
  │ Curso o proyecto          │ Rol      │ Contratadas │ Reportadas │
  │ Desarrollo en la nube ... │ Monitor  │     6 h     │    5 h     │
  │                     Total │          │     6 h     │    5 h     │
  └───────────────────────────┴──────────┴─────────────┴────────────┘

Detalle de tareas
  Curso: Desarrollo en la nube (Monitor)
  - Revisión de exámenes
      Estado: finalizado | Horas: 4
      Descripción: ...
  - Atención a estudiantes
      Estado: abierto | Horas: 1 | Reporte tardío
      Observaciones: Pendiente
                                                Página 1 de 1
```

---

## 2. Por qué

- **RF-14.4:** "el resultado del proceso debe generarse en formato PDF". **RN-42:** un PDF **individual** por persona.
- **RN-46:** el reporte sintetiza el trabajo y **no evalúa** el desempeño; el propio PDF lo aclara en una nota.
- **¿Por qué `fpdf`?** Es una librería en Go puro (no necesita un navegador ni programas externos como wkhtmltopdf), pequeña y suficiente para documentos con texto y tablas. La imagen del worker sigue siendo mínima.

---

## 3. Explicación detallada de `generador.go`

### 3.1 Entrada y configuración

```go
type Contenido struct {
	Datos       dominio.DatosSemanaPersona
	Resumen     string
	GeneradoEn  time.Time
	ProveedorIA string
	ModeloIA    string
}

type Generador struct {
	zona *time.Location
}

func NuevoGenerador(zona *time.Location) *Generador { ... }
```
- El PDF recibe los datos de la semana (paso 45), el resumen y quién lo generó.
- La **zona horaria** se usa para mostrar la fecha de generación en hora de Bogotá.

### 3.2 `Generar`

```go
func (g *Generador) Generar(c Contenido) ([]byte, error) {
	d := c.Datos
	doc := fpdf.New("P", "mm", "Letter", "")
	// Las fuentes estándar usan cp1252; el traductor convierte tildes y eñes.
	tr := doc.UnicodeTranslatorFromDescriptor("")
```
- `fpdf.New("P", "mm", "Letter", "")`: orientación vertical (*Portrait*), medidas en milímetros, papel carta.
- **Tildes y eñes:** las fuentes estándar de PDF (Helvetica) usan la codificación Windows-1252, no UTF-8. `tr` traduce cada texto antes de escribirlo; sin esto, "Página" saldría con caracteres extraños. Por eso **todo** texto pasa por `tr(...)`.

```go
	doc.SetTitle(...); doc.SetAuthor(...); doc.SetCreator(...)
	doc.SetMargins(20, 18, 20)
	doc.SetAutoPageBreak(true, 18)
	doc.AliasNbPages("")
	doc.SetFooterFunc(func() {
		doc.SetY(-14)
		...
		doc.CellFormat(0, 6, tr(fmt.Sprintf("Página %d de {nb}", doc.PageNo())), "", 0, "C", false, 0, "")
	})
	doc.AddPage()
```
- Metadatos del documento (título, autor), márgenes y **salto de página automático**: los textos largos continúan en la página siguiente.
- `AliasNbPages` + `{nb}`: fpdf reemplaza `{nb}` por el **total de páginas** al terminar ("Página 1 de 3").
- `SetFooterFunc` registra una función que fpdf ejecuta al final de **cada** página (pie de página).

```go
	titulo(doc, tr, "Resumen de la semana")
	doc.SetFont("Helvetica", "", 10.5)
	doc.MultiCell(0, 5.5, tr(c.Resumen), "", "J", false)
	...
	nota := "Resumen generado automáticamente a partir de las tareas registradas"
	if c.ProveedorIA != "" {
		nota += fmt.Sprintf(" (IA: %s / %s)", c.ProveedorIA, c.ModeloIA)
	}
	nota += ". Sintetiza el trabajo reportado y no constituye una evaluación de desempeño."
```
- `MultiCell` escribe texto que **se ajusta** al ancho y salta de línea; `"J"` lo justifica.
- La nota dice con qué modelo se generó (transparencia) y aclara que **no es una evaluación** (RN-46).

```go
	if err := doc.Error(); err != nil {
		return nil, fmt.Errorf("generando PDF: %w", err)
	}
	var buf bytes.Buffer
	if err := doc.Output(&buf); err != nil { ... }
	return buf.Bytes(), nil
```
- fpdf acumula los errores internamente; se revisan al final con `doc.Error()`.
- El PDF se arma **en memoria** (`bytes.Buffer`) y se devuelve como `[]byte`; quien lo use decidirá dónde guardarlo (el almacenamiento, paso 54).

### 3.3 Funciones auxiliares

- `titulo`, `filaDato`: estilo uniforme de títulos y pares "etiqueta: valor".
- `tablaDedicacion`: tabla con anchos fijos, encabezado sombreado y fila de **totales**, horas contratadas frente a reportadas por vinculación (RF-10.6 en papel).
- `detalleTarea`: título, estado, horas, marca de **reporte tardío**, descripción y observaciones, con sangría.
- `recortar`: si el nombre de un curso no cabe en su columna, lo acorta con "..." midiendo el ancho real del texto (`GetStringWidth`).
- `tipoEspacio` y `rolTexto`: textos legibles ("Proyecto", "Asistente graduado").
- `type traductor func(string) string`: tipo para pasar `tr` a las auxiliares.

### 3.4 Prueba

`TestGenerarProducePDFValido` genera el reporte **con** tareas (una descripción muy larga y un nombre de curso que no cabe en la tabla) y **sin** tareas, y comprueba que:
- empieza con `%PDF-` (la firma de todo archivo PDF);
- contiene `%%EOF` (el marcador de fin), es decir, está completo.

---

## 4. Conceptos de este paso

| Concepto | Explicación |
|---|---|
| `bytes.Buffer` | Un "archivo en memoria" donde se puede escribir. |
| Codificación de texto | UTF-8 (Go) frente a cp1252 (fuentes PDF estándar): hay que traducir. |
| Funciones registradas | `SetFooterFunc(func() {...})`: la librería la llama en cada página. |
| Errores acumulados | Algunas librerías guardan el primer error y se consulta al final (`doc.Error()`). |

---

## 5. Cómo comprobarlo

```bash
cd backend
go test ./internal/pdf/ -v
```

El PDF completo se podrá descargar en el paso 55, cuando el worker genere los primeros reportes.

---

## 6. Pasos para lograr esta versión desde el paso 49

1. `go get codeberg.org/go-pdf/fpdf@v0.12.0 && go mod tidy` (desde `backend/`).
2. Crear `backend/internal/pdf/generador.go` con `Contenido`, `Generador`, `NuevoGenerador`, `Generar` y las funciones auxiliares.
3. Crear `backend/internal/pdf/generador_test.go`.
4. `make pruebas lint`
5. Commit: `git add . && git commit -m "Paso 50: generador de PDF"`

---

## 7. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| RF-14.4 / RN-42 PDF individual | `Generar` produce un PDF por persona y semana. |
| RN-46 Sintetizar, no evaluar | Nota explícita en el documento. |
| RF-10.6 Horas contratadas frente a reportadas | Tabla de dedicación. |
