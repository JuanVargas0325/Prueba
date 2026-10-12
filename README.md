# Paso 61 · Newman en la integración continua

> Rama `paso-a-paso` · paso 61 de 65 · anterior: **Paso 60 · Colección de Postman** · siguiente: **Paso 62 · SonarQube**

---

## 1. Qué se hizo en este paso

Se agregó a GitHub Actions un tercer job, **`e2e`**. En cada push levanta **todo el sistema con Docker Compose**, como en producción, y ejecuta la colección de Postman del paso 60 con Newman.

| Archivo | Cambio |
|---|---|
| `.github/workflows/ci.yml` | **Modificado.** Job `e2e` y comentario de encabezado. |

```
calidad ──┐
          ├──► e2e
pruebas ──┘
```

---

## 2. Por qué

- Comprueba en cada cambio que **el despliegue completo funciona**: que el `Dockerfile` compila, que `docker-compose.yml` arranca, que las migraciones corren, que Nginx enruta y que el worker procesa reportes. Ninguna prueba anterior cubre todo eso junto.
- Es una evidencia objetiva para la entrega: el check verde en GitHub prueba que el sistema se despliega desde cero en una máquina limpia, no solo en la nuestra.

---

## 3. Explicación detallada del job

```yaml
  e2e:
    name: Despliegue con Docker Compose y pruebas de punta a punta (Newman)
    needs: [calidad, pruebas]
    runs-on: ubuntu-latest
    env:
      ADMIN_PASSWORD_CI: Admin-CI-2026*
```
- **`needs: [calidad, pruebas]`**: solo corre si los otros dos jobs pasaron. Así no se gastan minutos levantando contenedores cuando el código ni compila o tiene pruebas rotas. Sin `needs`, los jobs de GitHub Actions corren **en paralelo**.
- `ADMIN_PASSWORD_CI`: una clave solo para esta ejecución. No es secreta: la base de datos se destruye al final.

```yaml
      - name: Configuración de prueba (.env)
        run: |
          sed -e "s|^JWT_SECRETO=.*|JWT_SECRETO=$(openssl rand -hex 32)|" \
              -e "s|^POSTGRES_PASSWORD=.*|POSTGRES_PASSWORD=$(openssl rand -hex 16)|" \
              -e "s|^ADMIN_PASSWORD=.*|ADMIN_PASSWORD=${ADMIN_PASSWORD_CI}|" \
              .env.example > .env
```
- El `.env` no está en el repositorio (`.gitignore`, paso 1), así que se crea a partir de `.env.example`.
- `sed -e "s|patrón|reemplazo|"` reemplaza líneas. `^JWT_SECRETO=.*` significa "la línea que empieza con `JWT_SECRETO=` y todo lo que sigue". Se usa `|` como separador porque los valores podrían contener `/`.
- `openssl rand -hex 32` genera 32 bytes aleatorios en hexadecimal: un secreto distinto en cada ejecución. Así se comprueba que el sistema **no depende** de los valores de ejemplo.
- `\` al final de la línea continúa el comando en la siguiente.

```yaml
      - name: Levantar el sistema (construye las imágenes de la API y del worker)
        run: docker compose up --build -d --wait
```
- `--build` construye las imágenes; `-d` las deja en segundo plano; **`--wait`** espera hasta que todos los healthchecks (pasos 6 y 7) estén `healthy`. Si alguno nunca lo está, el comando falla y el job también.

```yaml
      - name: Ejecutar la colección de Postman con Newman
        run: |
          docker run --rm --network host -v "$PWD/postman:/etc/newman" postman/newman:ubuntu \
            run seguimiento.postman_collection.json \
            --env-var "baseUrl=http://localhost:8080" \
            --env-var "adminPassword=${ADMIN_PASSWORD_CI}"
```
- Igual que `make e2e` (paso 60), con una diferencia: en Linux, **`--network host`** hace que el contenedor comparta la red del runner, así que `localhost:8080` llega a Nginx. En Docker Desktop (Windows y Mac) se usa `host.docker.internal`.
- Si alguna comprobación falla, Newman termina con un código distinto de 0 y el job queda en rojo.

```yaml
      - name: Logs si algo falla
        if: failure()
        run: docker compose logs --no-color api worker

      - name: Detener el sistema
        if: always()
        run: docker compose down -v
```
- **`if: failure()`**: solo se ejecuta si un paso anterior falló, y muestra los logs para diagnosticar.
- **`if: always()`**: se ejecuta pase lo que pase. `-v` también borra los volúmenes.

**Relación con el paso 60:** los runners de GitHub usan la hora **UTC**. La corrección de zona horaria de la colección (hora de Bogotá) evita que este job falle los domingos por la noche.

---

## 4. Cómo comprobarlo

- Haga push y abra la pestaña **Actions** del repositorio: aparece el job "Despliegue con Docker Compose y pruebas de punta a punta (Newman)" después de los otros dos. Dentro se ve la tabla de Newman.
- Localmente, sin push: el mismo flujo es `docker compose up --build -d --wait && make e2e`.
- Validar la sintaxis del workflow (opcional) con `actionlint`:
  ```bash
  docker run --rm -v "$PWD:/repo" -w /repo rhysd/actionlint:latest
  ```

---

## 5. Pasos para lograr esta versión desde el paso 60

1. En `.github/workflows/ci.yml`, agregar al final, con la misma sangría que `calidad:` y `pruebas:`, el job `e2e` de la sección 3 (con sus cinco pasos).
2. Actualizar el comentario del encabezado (punto 3: e2e).
3. Commit y push: `git add . && git commit -m "Paso 61: Newman en la integración continua" && git push`
4. Revisar la pestaña *Actions*.

---

## 6. Preparación para las próximas entregas

- Este job es casi un **despliegue completo automatizado**. Para la entrega en la nube se puede agregar un job `despliegue` con `needs: e2e` que publique las imágenes (por ejemplo, en Artifact Registry) solo si todo pasó.
- La técnica de generar el `.env` con `sed` sirve también para configurar máquinas virtuales con un script de inicio.

---

## 7. Requisitos que atiende

| Requisito | Cómo |
|---|---|
| Despliegue con Docker Compose verificable | Se despliega desde cero en cada push. |
| Colección de Postman | Se ejecuta automáticamente. |
| Integración continua | Tercer job del pipeline. |
