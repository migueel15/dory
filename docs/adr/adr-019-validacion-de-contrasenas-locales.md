# ADR-019: Validación de contraseñas de cuentas locales

- Status: accepted
- Date: 2026-11-19

## Context

La primera cuenta de un nodo se puede crear en la aplicación. El comando `dory user create` también permite crear cuentas locales.

## Decision

Ambas vías aplican un mínimo de ocho puntos de código Unicode y un máximo de 4096 bytes. La aplicación pide confirmar la contraseña antes de enviar el formulario.

## Consequences

La política se aplica al crear cuentas. El inicio de sesión sigue verificando las credenciales que ya están guardadas.
