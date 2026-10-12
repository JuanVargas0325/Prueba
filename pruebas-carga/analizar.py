"""Resume los resultados de las pruebas de carga en tablas y gráficas Markdown.

Lee pruebas-carga/resultados/resumen.csv (generado por ejecutar.sh), agrupa las
repeticiones de cada escenario y nivel de concurrencia, y escribe
pruebas-carga/resultados/resumen.md con:
  - una tabla por escenario (promedio y desviación de las repeticiones),
  - gráficas de throughput y percentil 95 frente a la concurrencia (Mermaid).

Uso: python pruebas-carga/analizar.py
Solo usa la biblioteca estándar de Python.
"""
import csv
import statistics
from collections import defaultdict
from pathlib import Path

DIRECTORIO = Path(__file__).parent / "resultados"
NOMBRES = {
    "login": "E1 · Inicio de sesión (POST /api/v1/auth/login)",
    "seguimiento": "E2 · Vista principal del profesor (GET /api/v1/seguimiento/personas)",
    "tareas": "E3 · Registro de tareas (POST /api/v1/tareas)",
}


def media_y_desviacion(valores):
    media = statistics.mean(valores)
    desviacion = statistics.stdev(valores) if len(valores) > 1 else 0.0
    return media, desviacion


def cargar():
    grupos = defaultdict(list)
    with open(DIRECTORIO / "resumen.csv", encoding="utf-8") as archivo:
        for fila in csv.DictReader(archivo):
            grupos[(fila["escenario"], int(fila["concurrencia"]))].append(
                {k: float(v) for k, v in fila.items() if k != "escenario"})
    return grupos


def tabla(escenario, niveles, grupos):
    lineas = [
        f"#### {NOMBRES.get(escenario, escenario)}",
        "",
        "| Concurrencia | Peticiones/s | Media (ms) | p50 (ms) | p95 (ms) | p99 (ms) | Máx (ms) | Errores (%) | CPU API máx (%) | CPU BD máx (%) | Mem API máx (MB) |",
        "|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|",
    ]
    for c in niveles:
        corridas = grupos[(escenario, c)]
        rps = media_y_desviacion([r["rps"] for r in corridas])
        media = media_y_desviacion([r["media_ms"] for r in corridas])
        p95 = media_y_desviacion([r["p95_ms"] for r in corridas])
        completadas = sum(r["completadas"] for r in corridas)
        errores = sum(r["errores"] for r in corridas)
        porcentaje = 100 * errores / completadas if completadas else 0
        lineas.append(
            f"| {c} | {rps[0]:.1f} ± {rps[1]:.1f} | {media[0]:.1f} ± {media[1]:.1f} "
            f"| {statistics.mean(r['p50_ms'] for r in corridas):.0f} | {p95[0]:.0f} ± {p95[1]:.0f} "
            f"| {statistics.mean(r['p99_ms'] for r in corridas):.0f} | {max(r['max_ms'] for r in corridas):.0f} "
            f"| {porcentaje:.2f} | {max(r['cpu_api_max'] for r in corridas):.0f} "
            f"| {max(r['cpu_db_max'] for r in corridas):.0f} | {max(r['mem_api_max_mb'] for r in corridas):.0f} |")
    lineas.append("")
    lineas.append(f"Cada fila promedia {len(grupos[(escenario, niveles[0])])} corridas (± desviación estándar).")
    return lineas


def grafica(titulo, eje_y, escenario, niveles, grupos, metrica):
    valores = [statistics.mean(r[metrica] for r in grupos[(escenario, c)]) for c in niveles]
    maximo = max(valores) * 1.15 or 1
    return [
        "```mermaid",
        '%%{init: {"themeVariables": {"xyChart": {"plotColorPalette": "#1d4ed8"}}}}%%',
        "xychart-beta",
        f'    title "{titulo}"',
        f'    x-axis "Usuarios concurrentes" [{", ".join(str(c) for c in niveles)}]',
        f'    y-axis "{eje_y}" 0 --> {maximo:.0f}',
        f'    line [{", ".join(f"{v:.1f}" for v in valores)}]',
        "```",
    ]


def main():
    grupos = cargar()
    escenarios = sorted({e for e, _ in grupos}, key=lambda e: list(NOMBRES).index(e) if e in NOMBRES else 99)
    salida = []
    for escenario in escenarios:
        niveles = sorted(c for e, c in grupos if e == escenario)
        salida += tabla(escenario, niveles, grupos)
        salida.append("")
        salida += grafica(f"{escenario}: throughput", "Peticiones/s", escenario, niveles, grupos, "rps")
        salida.append("")
        salida += grafica(f"{escenario}: tiempo de respuesta p95", "ms", escenario, niveles, grupos, "p95_ms")
        salida.append("")
    (DIRECTORIO / "resumen.md").write_text("\n".join(salida), encoding="utf-8")
    print(f"Resumen escrito en {DIRECTORIO / 'resumen.md'}")


if __name__ == "__main__":
    main()
