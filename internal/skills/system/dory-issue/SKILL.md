---
name: dory-issue
description: "Report bugs or suggest improvements for Dory itself in migueel15/dory from the chat. Use only when the user describes a problem or feature request about Dory (UI, MCP, nodes, skills, apps), never for issues in the user's own project."
version: 1
---

# Crear issues en Dory

Usa esta skill cuando el usuario describa un fallo, comportamiento inesperado o propuesta de mejora **de Dory** (interfaz web, app de escritorio o móvil, MCP, malla de nodos, skills, arneses).

**No la uses** para bugs o tareas del propio proyecto en el que trabaja el usuario. Si hay duda, confirma si se refiere a Dory o a su repositorio.

Todas las issues en `migueel15/dory` se escriben **en español**.

## 1. Analizar primero

1. Intenta entender el problema y resolverlo si se trata de configuración o uso (por ejemplo, ajustes, permisos, estado de un nodo o cómo funciona una pantalla).
2. Si parece un bug real o una mejora de Dory, propón crear una issue en `migueel15/dory`.

## 2. Comprobar duplicados

Si `gh` está disponible y autenticado (`gh auth status`), busca issues similares antes de redactar una nueva:

```bash
gh issue list --repo migueel15/dory --state all --search "<palabras clave>" --limit 10
```

Si ya existe una issue equivalente, muéstrasela al usuario y pregúntale si prefiere comentar en esa o abrir una nueva.

## 3. Preparar el borrador

Elige la plantilla adecuada (bug o mejora) y rellénala con lo que ya sabes por la conversación. Pregunta al usuario **solo** los datos que falten y sean útiles (pasos para reproducir, versión de Dory, plataforma).

Muestra el título y el cuerpo completos del borrador al usuario antes de hacer nada.

### Plantilla de bug

```markdown
## Resumen
Descripción breve y clara del problema.

## Pasos para reproducir
1. ...
2. ...

## Resultado esperado
Qué debería ocurrir.

## Resultado actual
Qué ocurre en realidad.

## Entorno
- Versión de Dory:
- Plataforma: (web / escritorio / móvil)
- Sistema operativo:
- Arnés: (si aplica)

## Información adicional
Capturas, mensajes de error o contexto relevante (si aplica).
```

### Plantilla de mejora

```markdown
## Resumen
Descripción breve de la mejora propuesta.

## Problema o motivación
Qué necesidad resuelve o qué fricción existe hoy.

## Propuesta
Cómo debería funcionar.

## Alternativas
Otras opciones consideradas (opcional).

## Información adicional
Contexto adicional, ejemplos o referencias (opcional).
```

## 4. Privacidad y estilo

- **Privacidad**: no incluyas rutas personales del sistema de archivos, tokens, contraseñas, correos electrónicos, nombres de repositorios privados ni datos de proyectos del usuario, salvo que el usuario lo pida explícitamente.
- **Sin atribución**: no incluyas menciones al agente ni líneas de atribución automática en la issue.

## 5. Confirmar y crear

**Nunca publiques ni abras una issue sin la confirmación explícita del usuario** tras revisar el borrador.

Una vez confirmado:

- **Con `gh` autenticado** (`gh auth status` correcto): crea la issue escribiendo el cuerpo en un fichero temporal y pásalo con `--body-file`:
  ```bash
  gh issue create --repo migueel15/dory --title "<título>" --body-file <fichero>
  ```
  Devuelve al usuario el enlace de la issue creada.
- **Sin `gh` o sin autenticar**: genera un enlace prellenado con `title` y `body` codificados en URL:
  ```
  https://github.com/migueel15/dory/issues/new?title=<título>&body=<cuerpo>
  ```
  Si la URL resultante supera ~8 KB, acorta el cuerpo (manteniendo resumen, pasos y entorno) e indica al usuario qué detalle añadir al enviarla.
