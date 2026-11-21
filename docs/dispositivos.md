# Dispositivos Android

El panel lateral de un hilo tiene una pestaña **Dispositivo** que muestra en directo un emulador Android (AVD) o un móvil conectado por ADB, con cable o por Wi-Fi, y permite controlarlo. Todo corre en el nodo del hilo: el navegador o la app de escritorio solo reciben el vídeo y envían la entrada a través del servidor, así que también funciona en remoto.

El nodo hace de puente entre `adb`, `scrcpy-server` (v3.3.1, embebido en el binario) y el navegador por un WebSocket con la misma autenticación que la API. No hace falta instalar nada en el móvil.

## Requisitos

- **Nodo**: `adb` (platform-tools). Se busca en `ANDROID_HOME`, `ANDROID_SDK_ROOT`, `~/Android/Sdk` y `PATH`. Para emuladores hacen falta además el paquete `emulator`, al menos un AVD creado y KVM (`/dev/kvm`). Los emuladores arrancan con `-no-window`, así que sirven en nodos sin interfaz.
- **Móvil**: depuración USB o inalámbrica activada y la huella del equipo aceptada.
- **Navegador**: WebCodecs, que exige HTTPS o `localhost`. Si falta, el panel lo explica.

## Uso

- **+ → Dispositivo** abre una pestaña. Se pueden tener varias, y se renombran con doble clic. Solo la pestaña visible transmite; al cambiar de pestaña o cerrar el panel se corta el vídeo (el nodo espera unos segundos por si vuelves).
- Un toque, arrastre, rueda y teclado sobre el lienzo se envían al dispositivo; la barra tiene Atrás, Inicio, Recientes, bloqueo y volumen. Si el dispositivo se desconecta, la pestaña lo indica y se recupera sola al volver.
- **Iniciar** arranca un AVD y espera a que Android termine de iniciar. **Apagar** cierra un emulador; en un móvil físico pide confirmación.
- **Herramientas** (icono de ajustes en la barra): modo oscuro, tamaño de texto, orientación, Wi-Fi y datos, permisos de una app y, solo en emuladores, ubicación simulada. Tras cada cambio se muestra el valor leído del dispositivo. La app en primer plano se ve arriba.
- **Flotar sobre el chat** saca el dispositivo a una ventana que se mueve (barra superior) y se redimensiona (esquina); **Abrir en el panel lateral** lo devuelve.

## Conectar por Wi-Fi

1. En el móvil: Ajustes → Opciones de desarrollador → **Depuración inalámbrica**.
2. La primera vez: «Vincular dispositivo con código». En el panel, **Conectar por Wi-Fi**, escribe la dirección `IP:puerto` de vinculación y el código, y pulsa **Vincular**.
3. Escribe la dirección `IP:puerto` que aparece en la pantalla principal de Depuración inalámbrica y pulsa **Conectar**.

Los dispositivos conectados por Wi-Fi tienen un botón **Desconectar**. Emparejar y conectar solo se puede hacer desde una sesión autenticada.

## Agentes

Con el ajuste **Los agentes pueden usar dispositivos** (Ajustes → Tareas e hilos → Dispositivos, desactivado por defecto) el MCP del nodo ofrece herramientas `device_*`; con el ajuste apagado no aparecen. Sus llamadas se ven como tarjetas en el hilo y el dispositivo se abre en el panel para que el usuario vea lo que ocurre. Ver [MCP](mcp.md).

Un agente con el móvil personal puede enviar mensajes o tocar datos reales: activa el ajuste solo en nodos donde lo quieras. Los agentes no pueden apagar móviles físicos.

## Límites

- `device_ui` (uiautomator) tarda de 1 a 3 s y falla en pantallas protegidas o animadas; en ese caso el agente usa la captura y las coordenadas.
- El texto no ASCII solo se puede escribir con el dispositivo abierto en el panel.
- El vídeo se limita a 1080 px y 6 Mbit/s por defecto.
