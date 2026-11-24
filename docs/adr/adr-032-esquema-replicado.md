# ADR-032: Registro del esquema replicado y tipos web generados

- Status: accepted
- Date: 2026-11-24

## Context

El registro de operaciones y la capa de datos repetían los nombres de las tablas y columnas replicadas. La web mantenía a mano tipos que salían de los structs Go de la API.

## Decision

`internal/schema` define las entidades replicadas, sus claves, columnas, valores por defecto de `Put` y reglas de conflicto. El registro de operaciones y la capa de datos consultan esa definición. Las migraciones siguen definiendo el esquema físico de SQLite.

`cmd/gen-ts` genera las declaraciones TypeScript a partir de los structs Go expuestos por la API. El generador conserva las uniones de literales que los tipos Go representan como cadenas.

## Consequences

Una prueba compara el registro con la base de datos migrada. Al cambiar un struct expuesto a la web, ejecuta `go generate ./cmd/gen-ts` y guarda también `web/src/api/types.gen.ts`. Una prueba comprueba que el archivo coincide con la salida del generador.
