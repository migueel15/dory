package net.dorlab.dory;

import android.content.Context;
import android.net.Uri;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.util.Arrays;
import java.util.Comparator;
import java.util.HashMap;
import java.util.Locale;
import java.util.Map;
import java.util.regex.Pattern;
import java.util.zip.GZIPInputStream;
import javax.net.ssl.HttpsURLConnection;

/**
 * Disk cache for immutable Vite /assets/* files served by paired nodes.
 * Chromium skips its HTTP disk cache for responses with SSL cert errors
 * (including self-signed certs accepted via onReceivedSslError), so this
 * intercepts /assets/* requests and caches them locally with LRU eviction.
 */
final class AssetCache {
    static final long DEFAULT_MAX_BYTES = 40L * 1024 * 1024;
    private static final String PREFIX = "/assets/";
    private static final Pattern SAFE_NAME = Pattern.compile("^[A-Za-z0-9._-]+$");

    private final File dir;
    private final long maxBytes;

    AssetCache(Context context) {
        this(new File(context.getCacheDir(), "web-assets"), DEFAULT_MAX_BYTES);
    }

    AssetCache(File dir, long maxBytes) {
        this.dir = dir;
        this.maxBytes = maxBytes;
    }

    static String assetName(String path) {
        if (path == null || !path.startsWith(PREFIX)) return null;
        String name = path.substring(PREFIX.length());
        if (name.isEmpty() || name.contains("..") || !SAFE_NAME.matcher(name).matches()) return null;
        return name;
    }

    static String mimeType(String name) {
        String lower = name.toLowerCase(Locale.ROOT);
        if (lower.endsWith(".js") || lower.endsWith(".mjs")) return "application/javascript";
        if (lower.endsWith(".css")) return "text/css";
        if (lower.endsWith(".woff2")) return "font/woff2";
        if (lower.endsWith(".woff")) return "font/woff";
        if (lower.endsWith(".ttf")) return "font/ttf";
        if (lower.endsWith(".svg")) return "image/svg+xml";
        if (lower.endsWith(".png")) return "image/png";
        if (lower.endsWith(".wasm")) return "application/wasm";
        return "application/octet-stream";
    }

    static String charset(String mime) {
        if ("application/javascript".equals(mime)
            || "text/css".equals(mime)
            || "image/svg+xml".equals(mime)
            || (mime != null && mime.startsWith("text/"))) {
            return "UTF-8";
        }
        return null;
    }

    File get(String name) {
        File file = new File(dir, name);
        if (!file.isFile()) return null;
        file.setLastModified(System.currentTimeMillis());
        return file;
    }

    File put(String name, InputStream in) throws IOException {
        if (!dir.exists() && !dir.mkdirs() && !dir.isDirectory()) {
            throw new IOException("cannot create cache dir: " + dir);
        }
        File target = new File(dir, name);
        File tmp = new File(dir, name + ".tmp." + Thread.currentThread().getId() + "." + System.nanoTime());
        boolean renamed = false;
        try {
            try (FileOutputStream out = new FileOutputStream(tmp)) {
                byte[] buf = new byte[16384];
                int n;
                while ((n = in.read(buf)) > 0) {
                    out.write(buf, 0, n);
                }
            }
            if (target.exists() && !target.delete()) {
                // On POSIX renameTo atomically replaces target; fall through.
            }
            if (!tmp.renameTo(target)) {
                throw new IOException("failed to rename " + tmp + " to " + target);
            }
            renamed = true;
            target.setLastModified(System.currentTimeMillis());
            prune();
            return target;
        } finally {
            if (!renamed && tmp.exists()) {
                //noinspection ResultOfMethodCallIgnored
                tmp.delete();
            }
        }
    }

    synchronized void prune() {
        File[] files = dir.listFiles(f -> f.isFile() && !f.getName().contains(".tmp."));
        if (files == null || files.length == 0) return;
        long total = 0;
        for (File f : files) total += f.length();
        if (total <= maxBytes) return;
        Arrays.sort(files, Comparator.comparingLong(File::lastModified));
        for (File f : files) {
            if (total <= maxBytes) break;
            long len = f.length();
            if (f.delete()) total -= len;
        }
    }

    WebResourceResponse intercept(WebResourceRequest request) {
        if (request == null || !"GET".equalsIgnoreCase(request.getMethod())) return null;
        Uri uri = request.getUrl();
        if (uri == null || !"https".equalsIgnoreCase(uri.getScheme()) || uri.getHost() == null) return null;
        String name = assetName(uri.getPath());
        if (name == null) return null;

        int port = uri.getPort() == -1 ? 443 : uri.getPort();
        String pin = Pin.byHost.get(uri.getHost() + ":" + port);
        if (pin == null) return null;

        String origin = headerIgnoreCase(request.getRequestHeaders(), "Origin");

        File cached = get(name);
        if (cached != null) {
            return buildResponse(cached, name, origin);
        }

        HttpsURLConnection conn = null;
        try {
            conn = Pin.open(uri.toString(), pin, null);
            conn.setRequestMethod("GET");
            conn.setRequestProperty("Accept-Encoding", "gzip");
            int status = conn.getResponseCode();
            if (status != 200) return null;
            String encoding = conn.getContentEncoding();
            try (InputStream raw = conn.getInputStream();
                 InputStream in = encoding != null && encoding.toLowerCase(Locale.ROOT).contains("gzip")
                     ? new GZIPInputStream(raw) : raw) {
                File saved = put(name, in);
                return buildResponse(saved, name, origin);
            }
        } catch (Exception ignored) {
            return null;
        } finally {
            if (conn != null) conn.disconnect();
        }
    }

    private static WebResourceResponse buildResponse(File file, String name, String origin) {
        try {
            String mime = mimeType(name);
            String enc = charset(mime);
            Map<String, String> headers = new HashMap<>();
            headers.put("Cache-Control", "public, max-age=31536000, immutable");
            if (origin != null && !origin.isEmpty()) {
                headers.put("Access-Control-Allow-Origin", origin);
            }
            return new WebResourceResponse(mime, enc, 200, "OK", headers, new FileInputStream(file));
        } catch (IOException e) {
            return null;
        }
    }

    private static String headerIgnoreCase(Map<String, String> headers, String name) {
        if (headers == null) return null;
        for (Map.Entry<String, String> e : headers.entrySet()) {
            if (name.equalsIgnoreCase(e.getKey())) return e.getValue();
        }
        return null;
    }
}
