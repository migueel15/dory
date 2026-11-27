package net.dorlab.dory;

import android.content.Context;
import android.net.Uri;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.util.Arrays;
import java.util.Collections;
import java.util.Comparator;
import java.util.Locale;
import java.util.Map;
import javax.net.ssl.HttpsURLConnection;

/**
 * Disk cache for Vite's content-hashed /assets/* files of paired nodes.
 *
 * Chromium's WebView refuses to store HTTP cache entries or register Service
 * Workers for origins whose self-signed certificate was accepted via
 * onReceivedSslError. Intercepting immutable /assets/* requests here lets the
 * app paint from flash storage instead of re-downloading the whole bundle on
 * every open.
 */
final class AssetCache {
    private static final long MAX_BYTES = 40L * 1024 * 1024;
    private static final Map<String, String> HEADERS =
        Collections.singletonMap("Cache-Control", "public, max-age=31536000, immutable");

    static WebResourceResponse intercept(Context context, WebResourceRequest request) {
        if (!"GET".equalsIgnoreCase(request.getMethod())) return null;
        Uri uri = request.getUrl();
        if (uri == null || !"https".equalsIgnoreCase(uri.getScheme()) || uri.getHost() == null) return null;
        int port = uri.getPort() == -1 ? 443 : uri.getPort();
        String pin = Pin.byHost.get(uri.getHost() + ":" + port);
        if (pin == null) return null;

        String path = uri.getPath();
        if (path == null || !path.startsWith("/assets/")) return null;
        String name = path.substring("/assets/".length());
        if (name.isEmpty() || !isSafeName(name)) return null;

        String mime = mimeType(name);
        if (mime == null) return null;
        String encoding = isText(mime) ? "UTF-8" : null;

        File dir = new File(context.getCacheDir(), "web-assets");
        File file = new File(dir, name);
        if (file.isFile() && file.length() > 0) {
            try {
                // Touch mtime so active assets survive pruning.
                file.setLastModified(System.currentTimeMillis());
                return new WebResourceResponse(mime, encoding, 200, "OK", HEADERS, new FileInputStream(file));
            } catch (Exception ignored) {
                // Fall through to network fetch.
            }
        }

        HttpsURLConnection conn = null;
        try {
            conn = Pin.open(uri.toString(), pin, null);
            conn.setRequestMethod("GET");
            if (conn.getResponseCode() != 200) return null;
            byte[] data;
            try (InputStream in = conn.getInputStream(); ByteArrayOutputStream out = new ByteArrayOutputStream()) {
                byte[] buf = new byte[16384];
                int n;
                while ((n = in.read(buf)) > 0) out.write(buf, 0, n);
                data = out.toByteArray();
            }
            if (data.length == 0) return null;
            writeAtomically(dir, file, data);
            return new WebResourceResponse(mime, encoding, 200, "OK", HEADERS, new ByteArrayInputStream(data));
        } catch (Exception ignored) {
            return null;
        } finally {
            if (conn != null) conn.disconnect();
        }
    }

    private static boolean isSafeName(String name) {
        if (name.contains("..")) return false;
        for (int i = 0; i < name.length(); i++) {
            char c = name.charAt(i);
            boolean ok = (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
                || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_';
            if (!ok) return false;
        }
        return true;
    }

    private static String mimeType(String name) {
        String lower = name.toLowerCase(Locale.ROOT);
        if (lower.endsWith(".js") || lower.endsWith(".mjs")) return "application/javascript";
        if (lower.endsWith(".css")) return "text/css";
        if (lower.endsWith(".svg")) return "image/svg+xml";
        if (lower.endsWith(".png")) return "image/png";
        if (lower.endsWith(".woff2")) return "font/woff2";
        if (lower.endsWith(".woff")) return "font/woff";
        if (lower.endsWith(".json")) return "application/json";
        if (lower.endsWith(".wasm")) return "application/wasm";
        return null;
    }

    private static boolean isText(String mime) {
        return mime.startsWith("text/")
            || "application/javascript".equals(mime)
            || "application/json".equals(mime)
            || "image/svg+xml".equals(mime);
    }

    private static void writeAtomically(File dir, File target, byte[] data) {
        try {
            if (!dir.exists() && !dir.mkdirs()) return;
            File tmp = File.createTempFile("asset-", ".tmp", dir);
            try (FileOutputStream fos = new FileOutputStream(tmp)) {
                fos.write(data);
            }
            if (!tmp.renameTo(target)) tmp.delete();
            pruneIfNeeded(dir);
        } catch (Exception ignored) {
        }
    }

    private static void pruneIfNeeded(File dir) {
        File[] files = dir.listFiles();
        if (files == null || files.length == 0) return;
        long total = 0;
        for (File f : files) total += f.length();
        if (total <= MAX_BYTES) return;
        Arrays.sort(files, Comparator.comparingLong(File::lastModified));
        for (File f : files) {
            if (total <= MAX_BYTES * 3 / 4) break;
            long len = f.length();
            if (f.delete()) total -= len;
        }
    }
}
