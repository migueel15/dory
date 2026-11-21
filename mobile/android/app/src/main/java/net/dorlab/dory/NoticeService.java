package net.dorlab.dory;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.ServiceInfo;
import android.os.Build;
import android.os.IBinder;
import androidx.core.app.NotificationCompat;
import androidx.core.app.NotificationManagerCompat;
import androidx.core.content.ContextCompat;
import java.io.BufferedReader;
import java.io.InputStreamReader;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.LinkedHashSet;
import java.util.Map;
import java.util.Set;
import javax.net.ssl.HttpsURLConnection;
import org.json.JSONArray;
import org.json.JSONObject;

/**
 * Keeps the live stream of a paired node open, like Gotify does with its
 * server, and shows a native notification for every new notice of the mesh,
 * following the same rule as the interfaces: nothing while one has the focus,
 * unless the kind is set to always push.
 */
public class NoticeService extends Service {
    private static final int ONGOING = 1;
    private static final Duration FRESH = Duration.ofMinutes(10);
    private volatile boolean running;
    private volatile HttpsURLConnection stream;
    private Thread worker;

    /** Starts the service when there is a paired node; safe to call often. */
    static void start(Context context) {
        try {
            if (new JSONArray(prefs(context).getString("nodes", "[]")).length() == 0) return;
        } catch (Exception e) {
            return;
        }
        ContextCompat.startForegroundService(context, new Intent(context, NoticeService.class));
    }

    private static SharedPreferences prefs(Context context) {
        return context.getSharedPreferences("idea-capture", Context.MODE_PRIVATE);
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        channels();
        Notification ongoing = new NotificationCompat.Builder(this, "connection")
            .setSmallIcon(R.mipmap.ic_launcher).setContentTitle("Dory conectado a la malla")
            .setContentText("Recibe los avisos con la app cerrada.").setOngoing(true)
            .setPriority(NotificationCompat.PRIORITY_MIN).setContentIntent(openApp()).build();
        if (Build.VERSION.SDK_INT >= 34) startForeground(ONGOING, ongoing, ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE);
        else startForeground(ONGOING, ongoing);
        if (worker == null || !worker.isAlive()) {
            running = true;
            worker = new Thread(this::loop, "dory-notices");
            worker.start();
        }
        return START_STICKY;
    }

    @Override
    public void onDestroy() {
        running = false;
        HttpsURLConnection c = stream;
        if (c != null) c.disconnect();
        if (worker != null) worker.interrupt();
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    private void channels() {
        if (Build.VERSION.SDK_INT < 26) return;
        NotificationManager m = getSystemService(NotificationManager.class);
        m.createNotificationChannel(new NotificationChannel("notices", "Avisos", NotificationManager.IMPORTANCE_DEFAULT));
        m.createNotificationChannel(new NotificationChannel("connection", "Conexión con la malla", NotificationManager.IMPORTANCE_MIN));
    }

    private PendingIntent openApp() {
        Intent open = new Intent(this, MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP | Intent.FLAG_ACTIVITY_CLEAR_TOP);
        return PendingIntent.getActivity(this, 0, open, PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
    }

    /** Opens the node that sent the notice on the page it points to. */
    private PendingIntent openNotice(String id, JSONObject node, String url) {
        Intent open = new Intent(this, MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP | Intent.FLAG_ACTIVITY_CLEAR_TOP)
            .putExtra(MainActivity.NOTICE_NODE, node.toString()).putExtra(MainActivity.NOTICE_URL, url.startsWith("/") ? url : "/");
        return PendingIntent.getActivity(this, id.hashCode(), open, PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
    }

    /** Tries the paired nodes in turn; notices replicate, so any one is enough. */
    private void loop() {
        long backoff = 2_000;
        while (running) {
            JSONArray nodes;
            try {
                nodes = new JSONArray(prefs(this).getString("nodes", "[]"));
            } catch (Exception e) {
                nodes = new JSONArray();
            }
            if (nodes.length() == 0) { stopSelf(); return; }
            for (int i = 0; i < nodes.length() && running; i++) {
                JSONObject node = nodes.optJSONObject(i);
                if (node == null || node.optString("token").isEmpty() || node.optString("pin").isEmpty()) continue;
                try {
                    listen(node);
                    backoff = 2_000;
                } catch (Exception ignored) {
                    // Unreachable node or dropped stream: try the next one.
                }
            }
            try {
                Thread.sleep(backoff);
            } catch (InterruptedException e) {
                return;
            }
            backoff = Math.min(backoff * 2, 60_000);
        }
    }

    private void listen(JSONObject node) throws Exception {
        HttpsURLConnection c = Pin.open("https://" + node.getString("address") + "/api/events", node.getString("pin"), null);
        c.setRequestProperty("Authorization", "Bearer " + node.getString("token"));
        c.setRequestProperty("Accept", "text/event-stream");
        c.setConnectTimeout(10_000);
        c.setReadTimeout(60_000); // the node pings every 25 s
        stream = c;
        try {
            if (c.getResponseCode() != 200) throw new Exception("status " + c.getResponseCode());
            check(node);
            BufferedReader in = new BufferedReader(new InputStreamReader(c.getInputStream(), StandardCharsets.UTF_8));
            String event = "", line;
            while (running && (line = in.readLine()) != null) {
                if (line.startsWith("event:")) event = line.substring(6).trim();
                else if (line.startsWith("data:") && event.equals("change")
                    && "notice".equals(new JSONObject(line.substring(5).trim()).optString("entity"))) check(node);
                else if (line.isEmpty()) event = "";
            }
        } finally {
            c.disconnect();
        }
    }

    private String get(JSONObject node, String path) throws Exception {
        HttpsURLConnection c = Pin.open("https://" + node.getString("address") + path, node.getString("pin"), null);
        c.setConnectTimeout(10_000);
        c.setReadTimeout(15_000);
        try {
            return DoryPlugin.send(c, "GET", node.getString("token"), null);
        } finally {
            c.disconnect();
        }
    }

    /** Shows the fresh unread notices not shown yet; on connect it also shows the recent ones missed meanwhile. */
    private void check(JSONObject node) throws Exception {
        JSONArray notices = new JSONArray(get(node, "/api/notices"));
        Set<String> seen = new LinkedHashSet<>(prefs(this).getStringSet("seen-notices", new LinkedHashSet<>()));
        ArrayList<JSONObject> fresh = new ArrayList<>();
        for (int i = 0; i < notices.length(); i++) {
            JSONObject n = notices.getJSONObject(i);
            if (!n.isNull("read_at") || seen.contains(n.getString("id"))) continue;
            if (Build.VERSION.SDK_INT >= 26) try {
                if (Instant.parse(n.getString("created_at")).isBefore(Instant.now().minus(FRESH))) continue;
            } catch (Exception e) {
                continue;
            }
            fresh.add(n);
        }
        if (fresh.isEmpty()) return;
        Map<String, String> modes = new HashMap<>();
        JSONArray settings = new JSONArray(get(node, "/api/notice-settings"));
        for (int i = 0; i < settings.length(); i++) modes.put(settings.getJSONObject(i).getString("kind"), settings.getJSONObject(i).getString("mode"));
        boolean focus = new JSONObject(get(node, "/api/presence")).optBoolean("focus");
        for (JSONObject n : fresh) {
            seen.add(n.getString("id"));
            String mode = modes.getOrDefault(n.getString("kind"), "auto");
            if (mode.equals("off") || mode.equals("app") || (mode.equals("auto") && focus)) continue;
            show(n, node);
        }
        while (seen.size() > 300) seen.remove(seen.iterator().next());
        prefs(this).edit().putStringSet("seen-notices", seen).apply();
    }

    private void show(JSONObject n, JSONObject node) {
        if (!NotificationManagerCompat.from(this).areNotificationsEnabled()) return;
        // Same id as the interface's local notification, so one replaces the other.
        String id = n.optString("id");
        try {
            NotificationManagerCompat.from(this).notify(id.hashCode(), new NotificationCompat.Builder(this, "notices")
                .setSmallIcon(R.mipmap.ic_launcher).setContentTitle(n.optString("title")).setContentText(n.optString("body"))
                .setAutoCancel(true).setContentIntent(openNotice(id, node, n.optString("url"))).build());
        } catch (SecurityException ignored) {
            // Permission revoked meanwhile.
        }
    }
}
