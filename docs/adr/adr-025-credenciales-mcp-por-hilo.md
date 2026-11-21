# ADR-025 — Credenciales MCP por hilo gestionado

- Status: accepted
- Date: 2026-11-21

## Contexto

El runtime de ADR-008 entregaba a cada arnés el mismo secreto del nodo. El servidor MCP recibe el identificador del hilo en la URL para dirigir preguntas, servicios y acciones del panel lateral.

## Decisión

Dory deriva una credencial HMAC para cada hilo a partir de un secreto que conserva en memoria. La autenticación exige que la credencial coincida con el hilo de la URL y que las herramientas de vinculación usen ese mismo hilo.

## Consecuencias

Reiniciar el nodo invalida las credenciales de los procesos anteriores. Se conserva el alcance de escritura del MCP para tareas y memorias y los ajustes de disponibilidad del nodo. Los tokens de cuenta siguen sirviendo para clientes externos.
