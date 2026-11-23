package net.dorlab.dory;

import android.content.Context;
import android.content.Intent;
import android.app.role.RoleManager;
import android.provider.Settings;
import android.net.Uri;
import android.net.wifi.WifiManager;
import android.os.Build;
import android.webkit.CookieManager;
import com.getcapacitor.JSObject;
import com.getcapacitor.Plugin;
import com.getcapacitor.PluginCall;
import com.getcapacitor.PluginMethod;
import com.getcapacitor.annotation.CapacitorPlugin;
import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.DatagramPacket;
import java.net.InetAddress;
import java.net.MulticastSocket;
import java.nio.charset.StandardCharsets;
import java.util.List;
import java.util.Map;
import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;
import javax.net.ssl.HttpsURLConnection;
import org.json.JSONArray;
import org.json.JSONObject;

/** Native side of the launcher: discovery, pairing and opening a node. */
@CapacitorPlugin(name = "Dory")
public class DoryPlugin extends Plugin {
    private volatile Thread listener;
    private boolean downloadingAssistantModel;
    private WifiManager.MulticastLock lock;

    /** Listens for the UDP beacon every node sends on the local network. */
    @PluginMethod
    public void startDiscovery(PluginCall call) {
        if (listener != null) { call.resolve(); return; }
        WifiManager wifi = (WifiManager) getContext().getApplicationContext().getSystemService(Context.WIFI_SERVICE);
        lock = wifi.createMulticastLock("dory");
        lock.setReferenceCounted(false);
        lock.acquire();
        listener = new Thread(() -> {
            try (MulticastSocket s = new MulticastSocket(7711)) {
                s.joinGroup(InetAddress.getByName("239.255.77.10"));
                byte[] buf = new byte[1024];
                while (!Thread.currentThread().isInterrupted()) {
                    DatagramPacket p = new DatagramPacket(buf, buf.length);
                    s.receive(p);
                    JSONObject b = new JSONObject(new String(p.getData(), 0, p.getLength(), StandardCharsets.UTF_8));
                    if (!"dory".equals(b.optString("app"))) continue;
                    JSObject ev = new JSObject();
                    ev.put("id", b.optString("node"));
                    ev.put("name", b.optString("name"));
                    ev.put("address", p.getAddress().getHostAddress() + ":" + b.optInt("port", 7710));
                    notifyListeners("found", ev);
                }
            } catch (Exception ignored) {
            }
        });
        listener.setDaemon(true);
        listener.start();
        call.resolve();
    }

    /** Pairs with a node using the code it shows in Environments. */
    @PluginMethod
    public void pair(PluginCall call) {
        String address = call.getString("address"), code = call.getString("code"), name = call.getString("name", Build.MODEL);
        new Thread(() -> {
            try {
                String[] seen = new String[1];
                JSONObject body = new JSONObject().put("code", code).put("name", name);
                JSONObject out = new JSONObject(send(Pin.open("https://" + address + "/device/v1/pair", null, seen), "POST", null, body.toString()));
                String pin = seen[0];
                if (!out.optString("proof").equals(proof(code, pin + "|device")) || !pin.equals(out.optString("pin"))) {
                    call.reject("El nodo no demostró conocer el código: puede haber alguien en medio.");
                    return;
                }
                JSObject r = new JSObject();
                r.put("id", out.optString("node_id"));
                r.put("name", out.optString("name"));
                r.put("address", address);
                r.put("pin", pin);
                r.put("token", out.optString("token"));
                call.resolve(r);
            } catch (Exception e) {
                call.reject(message(e));
            }
        }).start();
    }

    /** Calls the API of a paired node with its token (e.g. /api/mesh). */
    @PluginMethod
    public void get(PluginCall call) {
        String address = call.getString("address"), pin = call.getString("pin"), token = call.getString("token"), path = call.getString("path");
        new Thread(() -> {
            try {
                JSObject r = new JSObject();
                r.put("body", send(Pin.open("https://" + address + path, pin, null), "GET", token, null));
                call.resolve(r);
            } catch (Exception e) {
                call.reject(message(e));
            }
        }).start();
    }

    /** Trades the device token for a web session cookie and returns the node origin. */
    static String session(String address, String pin, String token) throws Exception {
        HttpsURLConnection c = Pin.open("https://" + address + "/device/v1/session", pin, null);
        send(c, "POST", token, "");
        String origin = "https://" + address;
        CookieManager cm = CookieManager.getInstance();
        Map<String, List<String>> headers = c.getHeaderFields();
        List<String> cookies = headers.get("Set-Cookie");
        if (cookies == null) cookies = headers.get("set-cookie");
        if (cookies != null) for (String v : cookies) cm.setCookie(origin, v);
        cm.flush();
        Pin.byHost.put(address, pin);
        return origin;
    }

    /** Trades the token for a session and shows the node in the web view. */
    @PluginMethod
    public void open(PluginCall call) {
        String address = call.getString("address"), pin = call.getString("pin"), token = call.getString("token");
        new Thread(() -> {
            try {
                String origin = session(address, pin, token);
                getActivity().runOnUiThread(() -> getBridge().getWebView().loadUrl(origin + "/"));
                call.resolve();
            } catch (Exception e) {
                call.reject(message(e));
            }
        }).start();
    }

    @PluginMethod
    public void forget(PluginCall call) {
        String address = call.getString("address");
        if (address != null) Pin.byHost.remove(address);
        call.resolve();
    }

    @PluginMethod
    public void saveCaptureSettings(PluginCall call) {
        getActivity().runOnUiThread(() -> {
            Uri page = Uri.parse(String.valueOf(getBridge().getWebView().getUrl()));
            Uri launcher = Uri.parse(getBridge().getLocalUrl());
            if (!launcher.getScheme().equals(page.getScheme()) || !launcher.getAuthority().equals(page.getAuthority())) {
                call.reject("Abre el selector de entornos de Dory.");
                return;
            }
            JSONArray nodes = call.getArray("nodes");
            if (nodes == null || !CaptureNodes.save(getContext(), nodes, call.getString("destination", ""))) {
                call.reject("No se pudo guardar el nodo de captura.");
                return;
            }
            NoticeService.start(getContext());
            call.resolve();
        });
    }

    private boolean isLauncher() {
        Uri page = Uri.parse(String.valueOf(getBridge().getWebView().getUrl()));
        Uri launcher = Uri.parse(getBridge().getLocalUrl());
        return launcher.getScheme().equals(page.getScheme()) && launcher.getAuthority().equals(page.getAuthority());
    }

    @PluginMethod
    public void openAssistantSettings(PluginCall call) {
        getActivity().runOnUiThread(() -> {
            if (!isLauncher()) { call.reject("Abre el selector de entornos."); return; }
            try {
                if (Build.VERSION.SDK_INT >= 29) {
                    RoleManager roles = getContext().getSystemService(RoleManager.class);
                    if (roles != null && roles.isRoleAvailable(RoleManager.ROLE_ASSISTANT)
                        && !roles.isRoleHeld(RoleManager.ROLE_ASSISTANT)) {
                        getActivity().startActivity(roles.createRequestRoleIntent(RoleManager.ROLE_ASSISTANT));
                        call.resolve();
                        return;
                    }
                }
                getActivity().startActivity(new Intent(Settings.ACTION_MANAGE_DEFAULT_APPS_SETTINGS));
                call.resolve();
            } catch (Exception e) {
                call.reject("No se pudo abrir la configuración del asistente.");
            }
        });
    }

    @PluginMethod
    public void assistantModels(PluginCall call) {
        getActivity().runOnUiThread(() -> {
            if (!isLauncher()) { call.reject("Abre el selector de entornos."); return; }
            new Thread(() -> {
                JSObject result = new JSObject();
                result.put("supported", Build.VERSION.SDK_INT >= 26);
                result.put("selected", AssistantModels.selected(getContext()));
                result.put("tiny", AssistantModels.installed(getContext(), AssistantModels.TINY));
                result.put("small", AssistantModels.installed(getContext(), AssistantModels.SMALL));
                result.put("parakeet", AssistantModels.installed(getContext(), AssistantModels.PARAKEET));
                call.resolve(result);
            }, "assistant-model-status").start();
        });
    }

    @PluginMethod
    public void selectAssistantModel(PluginCall call) {
        getActivity().runOnUiThread(() -> {
            if (!isLauncher()) { call.reject("Abre el selector de entornos."); return; }
            String id = call.getString("model", "");
            new Thread(() -> {
                try {
                    AssistantModels.select(getContext(), id);
                    call.resolve();
                } catch (Exception e) { call.reject(message(e)); }
            }, "assistant-model-select").start();
        });
    }

    @PluginMethod
    public void downloadAssistantModel(PluginCall call) {
        getActivity().runOnUiThread(() -> {
            if (!isLauncher()) { call.reject("Abre el selector de entornos."); return; }
            String id = call.getString("model", "");
            if (Build.VERSION.SDK_INT < 26) { call.reject("Los modelos locales requieren Android 8 o posterior."); return; }
            if (!AssistantModels.valid(id) || AssistantModels.ANDROID.equals(id)) {
                call.reject("Modelo desconocido."); return;
            }
            if (downloadingAssistantModel) { call.reject("Ya hay una descarga en curso."); return; }
            downloadingAssistantModel = true;
            new Thread(() -> {
                try {
                    if (AssistantModels.PARAKEET.equals(id)) {
                        downloadParakeet();
                        call.resolve();
                        return;
                    }
                    new ai.moonshine.voice.AssetDownloader().ensureModelPresent(
                        AssistantModels.directory(getContext(), id), AssistantModels.spec(id),
                        (path, index, total, done, size) -> {
                            JSObject progress = new JSObject();
                            progress.put("model", id);
                            progress.put("file", index);
                            progress.put("files", total);
                            progress.put("percent", size > 0 ? (int) (100 * done / size) : -1);
                            notifyListeners("assistantModelProgress", progress);
                        });
                    call.resolve();
                } catch (Exception e) { call.reject(message(e)); }
                finally { getActivity().runOnUiThread(() -> downloadingAssistantModel = false); }
            }, "assistant-model-download").start();
        });
    }

    /** Downloads each Parakeet file to a temporary name so a cut download never looks installed. */
    private void downloadParakeet() throws Exception {
        java.io.File dir = AssistantModels.directory(getContext(), AssistantModels.PARAKEET);
        if (!dir.isDirectory() && !dir.mkdirs()) throw new Exception("No se pudo crear la carpeta del modelo.");
        String[] files = AssistantModels.PARAKEET_FILES;
        for (int index = 0; index < files.length; index++) {
            java.io.File target = new java.io.File(dir, files[index]);
            if (target.isFile()) continue;
            java.io.File part = new java.io.File(dir, files[index] + ".part");
            HttpsURLConnection c = (HttpsURLConnection) new java.net.URL(AssistantModels.PARAKEET_URL + files[index]).openConnection();
            c.setConnectTimeout(15000);
            c.setReadTimeout(30000);
            int status = c.getResponseCode();
            if (status >= 400) throw new Exception("La descarga respondió " + status + ".");
            long size = c.getContentLengthLong();
            long done = 0;
            int last = -1;
            try (InputStream in = c.getInputStream(); java.io.FileOutputStream out = new java.io.FileOutputStream(part)) {
                byte[] b = new byte[1 << 16];
                int n;
                while ((n = in.read(b)) > 0) {
                    out.write(b, 0, n);
                    done += n;
                    int percent = size > 0 ? (int) (100 * done / size) : -1;
                    if (percent != last) {
                        last = percent;
                        JSObject progress = new JSObject();
                        progress.put("model", AssistantModels.PARAKEET);
                        progress.put("file", index + 1);
                        progress.put("files", files.length);
                        progress.put("percent", percent);
                        notifyListeners("assistantModelProgress", progress);
                    }
                }
            } finally { c.disconnect(); }
            if (size > 0 && done != size) { part.delete(); throw new Exception("La descarga se cortó."); }
            if (!part.renameTo(target)) throw new Exception("No se pudo guardar el modelo.");
        }
    }

    static String send(HttpsURLConnection c, String method, String token, String body) throws Exception {
        c.setRequestMethod(method);
        if (token != null) c.setRequestProperty("Authorization", "Bearer " + token);
        if (body != null) {
            c.setDoOutput(true);
            c.setRequestProperty("Content-Type", "application/json");
            try (OutputStream o = c.getOutputStream()) { o.write(body.getBytes(StandardCharsets.UTF_8)); }
        }
        int status = c.getResponseCode();
        InputStream in = status < 400 ? c.getInputStream() : c.getErrorStream();
        ByteArrayOutputStream buf = new ByteArrayOutputStream();
        if (in != null) { byte[] b = new byte[8192]; int n; while ((n = in.read(b)) > 0) buf.write(b, 0, n); in.close(); }
        String text = buf.toString("UTF-8");
        if (status == 401 || status == 403) throw new Exception(status == 403 ? "Código incorrecto o caducado." : "El nodo ya no reconoce este móvil: empareja de nuevo.");
        if (status >= 400) throw new Exception("El nodo respondió " + status + ": " + text.trim());
        return text;
    }

    /** Same HMAC the node computes: hex(HMAC-SHA256(code, pin|device)). */
    private static String proof(String code, String msg) throws Exception {
        Mac mac = Mac.getInstance("HmacSHA256");
        mac.init(new SecretKeySpec(code.getBytes(StandardCharsets.UTF_8), "HmacSHA256"));
        StringBuilder sb = new StringBuilder();
        for (byte b : mac.doFinal(msg.getBytes(StandardCharsets.UTF_8))) sb.append(String.format("%02x", b));
        return sb.toString();
    }

    private static String message(Exception e) {
        String m = e.getMessage();
        return m == null || m.isEmpty() ? e.getClass().getSimpleName() : m;
    }

    @Override
    protected void handleOnDestroy() {
        if (listener != null) listener.interrupt();
        if (lock != null && lock.isHeld()) lock.release();
    }
}
