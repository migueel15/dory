# ADR-035: Repositorios tipados para las entidades replicadas

- Status: accepted
- Date: 2026-11-24

## Context

Guardar una fila completa podía recuperar valores obsoletos o perder cambios de otro escritor. El esquema replicado ya centraliza las columnas, sus tipos y sus valores por defecto.

## Decision

Los repositorios de `internal/data` aplican los valores por defecto al crear una fila y actualizan solo las columnas presentes en un parche. `PatchIf` comprueba `updated_at` dentro de la transacción. `Upsert` queda reservado a las entidades con una identidad natural.

El oplog serializa los escritores con transacciones inmediatas y notifica los cambios tras el commit. Las operaciones compuestas que necesitan consistencia local, como crear la tarea de un hilo y enlazarla, comparten una transacción.

## Consequences

`POST` en las rutas genéricas crea un recurso. `PUT` y `PATCH` actualizan las claves presentes, permiten `null` en columnas anulables y aceptan una versión opcional mediante `If-Match` o `updated_at`. Un identificador duplicado o una versión obsoleta devuelve HTTP 409. Los planes conservan su revisión numérica de edición.

`Store.LocalDB()` se reserva a las tablas locales de cada paquete. Los avisos leídos se archivan por el oplog al vencer su plazo de retención y la malla replica el archivado.
