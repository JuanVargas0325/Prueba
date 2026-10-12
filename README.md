# Paso 62 · Análisis de calidad con SonarQube

> Rama `paso-a-paso` · paso 62 de 65 · anterior: **Paso 61 · Newman en la integración continua** · siguiente: **Paso 63 · Scripts de pruebas de carga**

---

## 1. Qué se hizo en este paso

Se agregó el análisis con **SonarQube** a la integración continua. En cada push se mide la calidad del código (errores probables, vulnerabilidades, código duplicado, cobertura de pruebas) y se evalúa su **Quality Gate**.

| Archivo | Cambio |
|---|---|
| `sonar-project.properties` | **Nuevo.** Qué analizar, qué excluir y dónde está la cobertura. |
| `.github/workflows/ci.yml` | **Modificado.** Cobertura entre paquetes (`-coverpkg`), artefacto `cobertura`, job `sonarqube` y encabezado. **Queda igual que en `main`.** |

```
calidad ──┐
          ├──► e2e
pruebas ──┤
          └──► sonarqube
```

---

## 2. Por qué

- **La entrega lo exige**: un reporte de SonarQube del backend.
- SonarQube revisa reglas que el linter (paso 8) no cubre del todo: **complejidad cognitiva** (funciones difíciles de leer), **duplicación**, **hotspots de seguridad** (código que una persona debe revisar, como el manejo de secretos) y la **cobertura** de pruebas, todo en un tablero.
- El **Quality Gate** es un umbral automático, por ejemplo: "el código nuevo no tiene errores y tiene al menos 80 % de cobertura". Si no se cumple, el job falla.

---

## 3. Explicación detallada

### 3.1 `sonar-project.properties`

SonarQube lee este archivo de la raíz del repositorio.

```properties
sonar.projectKey=seguimiento
# sonar.organization=nombre-de-la-organizacion
sonar.projectName=Plataforma de seguimiento de monitores y asistentes
sonar.sourceEncoding=UTF-8
```
- `projectKey`: identificador único del proyecto en el servidor. `organization` solo se usa en SonarQube Cloud.
- `UTF-8`: los comentarios tienen tildes y eñes.

```properties
sonar.sources=backend
sonar.exclusions=**/*_test.go,**/vendor/**,backend/internal/basedatos/migraciones/**
sonar.tests=backend
sonar.test.inclusions=**/*_test.go
```
- Se analiza `backend/`. Los archivos `_test.go` se clasifican como **pruebas**: no cuentan como código de producción, ni para buscar defectos ni para la cobertura.
- Las migraciones SQL se excluyen porque son esquema, no lógica.
- `**` significa "cualquier carpeta, a cualquier profundidad".

```properties
sonar.go.coverage.reportPaths=backend/coverage.out
sonar.coverage.exclusions=backend/cmd/**
```
- SonarQube **no ejecuta pruebas**: lee el reporte de cobertura que genera `go test -coverprofile`.
- `cmd/` (los `main` de la API y del worker, que solo conectan piezas) se excluye de la cobertura: se prueba con el job e2e (paso 61), que la herramienta de cobertura no mide.

### 3.2 Cambios en el job `pruebas`

```yaml
      - name: Pruebas con cobertura
        # -coverpkg mide la cobertura entre paquetes (por ejemplo, las migraciones
        # que ejercitan las pruebas de integración de los repositorios).
        run: go test -race -tags integracion -covermode=atomic -coverpkg=./internal/... -coverprofile=coverage.out ./...

      - uses: actions/upload-artifact@v4
        with:
          name: cobertura
          path: backend/coverage.out
```
- Sin `-coverpkg`, Go solo cuenta las líneas que las pruebas de un paquete ejecutan **en ese mismo paquete**. Con `-coverpkg=./internal/...` también cuentan las líneas de otros paquetes. Por ejemplo, las pruebas de `repositorio` ejecutan el código de `basedatos` (migraciones). Así la cifra refleja mejor lo que está probado.
- `upload-artifact` guarda `coverage.out`. Cada job de GitHub Actions corre en **otra máquina**, así que el archivo hay que pasarlo explícitamente al job `sonarqube`, que lo baja con `download-artifact`.

### 3.3 El job `sonarqube`: dos modos

```yaml
  sonarqube:
    needs: pruebas
    env:
      SONAR_TOKEN: ${{ secrets.SONAR_TOKEN }}
      SONAR_HOST_URL: ${{ vars.SONAR_HOST_URL }}
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0 # SonarQube necesita el historial completo
```
- `needs: pruebas`: espera el reporte de cobertura.
- `fetch-depth: 0` descarga todo el historial de git. SonarQube lo usa para saber qué código es **nuevo** (el Quality Gate se enfoca en él) y quién escribió cada línea.
- `secrets.*` y `vars.*` se configuran en GitHub (*Settings → Secrets and variables → Actions*). Si no existen, quedan vacíos.

**Modo 1, con servidor (si hay `SONAR_TOKEN`):**
```yaml
      - name: Escanear con el servidor configurado
        if: env.SONAR_TOKEN != ''
        uses: SonarSource/sonarqube-scan-action@v5
        with:
          args: -Dsonar.qualitygate.wait=true
```
Envía el análisis a SonarQube Cloud o al servidor del curso y **espera** el resultado del Quality Gate (`qualitygate.wait`).

**Modo 2, sin configurar nada (el que se usa por defecto):**
```yaml
      - name: Levantar SonarQube Community temporal
        if: env.SONAR_TOKEN == ''
        run: |
          docker run -d --name sonarqube -p 9000:9000 sonarqube:community
          for _ in $(seq 1 60); do
            estado=$(curl -s localhost:9000/api/system/status | jq -r .status 2>/dev/null || true)
            [ "$estado" = "UP" ] && break
            sleep 5
          done
          token=$(curl -s -u admin:admin -X POST "localhost:9000/api/user_tokens/generate?name=ci&type=GLOBAL_ANALYSIS_TOKEN" | jq -r .token)
          echo "::add-mask::$token"
          echo "SONAR_LOCAL_TOKEN=$token" >> "$GITHUB_ENV"
```
- Levanta un **servidor SonarQube dentro del runner** con Docker. Así no hace falta crear una cuenta.
- El `for` consulta cada 5 s (hasta 5 minutos) si el servidor ya está `UP`. `jq -r .status` extrae el campo `status` del JSON.
- Con el usuario por defecto `admin:admin` genera un token de análisis.
- `::add-mask::` le pide a GitHub ocultar ese valor en los logs. Escribir en `$GITHUB_ENV` hace que la variable exista en los pasos siguientes.

```yaml
      - name: Escanear con SonarQube Community
        run: |
          docker run --rm --network host \
            -e SONAR_HOST_URL=http://localhost:9000 -e SONAR_TOKEN="$SONAR_LOCAL_TOKEN" \
            -v "$PWD:/usr/src" sonarsource/sonar-scanner-cli -Dsonar.qualitygate.wait=true
```
El **scanner** (otro contenedor) lee el código montado en `/usr/src`, aplica `sonar-project.properties` y envía el análisis al servidor.

```yaml
      - name: Exportar el reporte de SonarQube
        if: always() && env.SONAR_TOKEN == ''
        run: |
          ...curl .../qualitygates/project_status  > quality-gate.json
          ...curl .../measures/component           > metricas.json
          ...curl .../issues/search                > hallazgos.json
          ...curl .../hotspots/search              > hotspots.json
          { ...tabla en Markdown... } | tee reporte-sonarqube/resumen.md >> "$GITHUB_STEP_SUMMARY"
      - uses: actions/upload-artifact@v4
        with:
          name: reporte-sonarqube
```
- Como el servidor temporal **desaparece** al terminar el job, se descarga su información por la API web de SonarQube y se guarda como **artefacto** `reporte-sonarqube`.
- `$GITHUB_STEP_SUMMARY` muestra el resumen (Quality Gate, métricas y hallazgos) en la **página del run** de GitHub, sin descargar nada.
- `if: always()`: el reporte se exporta incluso si el Quality Gate falló, que es justo cuando más interesa.

---

## 4. Cómo comprobarlo

1. Haga push y abra *Actions* → el run → job **Análisis SonarQube**.
2. En la página del run (pestaña *Summary*) aparece la tabla "SonarQube" con el Quality Gate y las métricas.
3. Abajo, en *Artifacts*, descargue **reporte-sonarqube**. Ese es el reporte para la entrega.

Localmente (opcional, ~3 GB de imágenes):
```bash
docker run -d --name sonarqube -p 9000:9000 sonarqube:community
# entrar a http://localhost:9000 (admin/admin), crear un token y:
cd backend && go test -tags integracion -coverpkg=./internal/... -coverprofile=coverage.out ./... && cd ..
docker run --rm --network host -e SONAR_HOST_URL=http://localhost:9000 -e SONAR_TOKEN=<token> -v "$PWD:/usr/src" sonarsource/sonar-scanner-cli
```

---

## 5. Pasos para lograr esta versión desde el paso 61

1. Crear `sonar-project.properties` en la raíz (sección 3.1).
2. En `ci.yml`, job `pruebas`: agregar `-coverpkg=./internal/...` al `go test` y el paso `upload-artifact` (sección 3.2).
3. Agregar el job `sonarqube` entre `pruebas` y `e2e` (sección 3.3).
4. Actualizar el encabezado del `ci.yml`.
5. Commit y push: `git add . && git commit -m "Paso 62: SonarQube" && git push`
6. Descargar el artefacto `reporte-sonarqube`.

---

## 6. Preparación para las próximas entregas

- **Si el curso da un servidor de SonarQube o se usa SonarQube Cloud**, no hay que cambiar el workflow: basta con crear el secreto `SONAR_TOKEN` y la variable `SONAR_HOST_URL` en GitHub (y `sonar.organization` en Cloud). Así el historial de calidad queda guardado entre entregas.
- Cada entrega puede adjuntar su `reporte-sonarqube` para mostrar cómo evoluciona la calidad.

---

## 7. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| Reporte de SonarQube del backend (entregables de la Entrega 1) | Artefacto `reporte-sonarqube` y resumen en el run. |
| Calidad del código | Quality Gate en cada push. |
