package handler

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"seguimiento/internal/apperr"
	"seguimiento/internal/servicio"
)

// campoAdjuntos es el nombre del campo multipart que trae los archivos.
const campoAdjuntos = "adjuntos"

// limitarCuerpo impide leer peticiones más grandes que el máximo indicado.
func limitarCuerpo(c *gin.Context, maxBytes int64) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
}

// esMultipart indica si la petición es un formulario con archivos.
func esMultipart(c *gin.Context) bool {
	return strings.HasPrefix(c.ContentType(), "multipart/form-data")
}

// leerArchivos abre los archivos del campo "adjuntos" de un formulario
// multipart. El llamador debe invocar la función devuelta para cerrarlos.
func leerArchivos(form *multipart.Form) ([]servicio.ArchivoAdjunto, func(), error) {
	var abiertos []multipart.File
	cerrar := func() {
		for _, f := range abiertos {
			_ = f.Close()
		}
	}
	var archivos []servicio.ArchivoAdjunto
	for _, cabecera := range form.File[campoAdjuntos] {
		f, err := cabecera.Open()
		if err != nil {
			cerrar()
			return nil, func() {}, apperr.Validacion("no fue posible leer el archivo %q", cabecera.Filename)
		}
		abiertos = append(abiertos, f)
		archivos = append(archivos, servicio.ArchivoAdjunto{Nombre: cabecera.Filename, Contenido: f})
	}
	return archivos, cerrar, nil
}

// formularioMultipart interpreta el formulario de la petición.
func formularioMultipart(c *gin.Context) (*multipart.Form, error) {
	form, err := c.MultipartForm()
	if err != nil {
		var demasiadoGrande *http.MaxBytesError
		if errors.As(err, &demasiadoGrande) {
			return nil, err
		}
		return nil, apperr.Validacion("formulario multipart inválido")
	}
	return form, nil
}

// servirArchivo envía un archivo como descarga. Siempre se entrega como
// adjunto y con nosniff, para que el navegador no ejecute su contenido.
func servirArchivo(c *gin.Context, contenido io.Reader, tamano int64, tipo, nombre string) {
	c.DataFromReader(http.StatusOK, tamano, tipo, contenido, map[string]string{
		"Content-Disposition":    disposicionAdjunto(nombre),
		"X-Content-Type-Options": "nosniff",
		"Cache-Control":          "private, no-store",
	})
}

// disposicionAdjunto arma la cabecera Content-Disposition con un nombre ASCII
// seguro y el nombre original codificado en UTF-8 (RFC 6266).
func disposicionAdjunto(nombre string) string {
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, nombreASCII(nombre), url.PathEscape(nombre))
}

// nombreASCII elimina tildes y caracteres no seguros de un nombre de archivo.
func nombreASCII(nombre string) string {
	sinTildes, _, _ := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), nombre)
	var b strings.Builder
	for _, r := range sinTildes {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "archivo"
	}
	return b.String()
}

// nombreArchivo convierte un nombre de persona en un texto seguro para un
// nombre de archivo: sin tildes, en minúsculas y con guiones.
func nombreArchivo(nombre string) string {
	sinTildes, _, _ := transform.String(transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC), nombre)
	var b strings.Builder
	for _, r := range strings.ToLower(sinTildes) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
				b.WriteByte('-')
			}
		}
	}
	resultado := strings.Trim(b.String(), "-")
	if resultado == "" {
		return "persona"
	}
	return resultado
}
