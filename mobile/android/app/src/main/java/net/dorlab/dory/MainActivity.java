package net.dorlab.dory;

import android.net.Uri;
import android.Manifest;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Intent;
import android.os.Build;
import android.webkit.JavascriptInterface;
import androidx.core.app.NotificationCompat;
import androidx.core.app.NotificationManagerCompat;
import android.content.pm.PackageManager;
import android.net.http.SslError;
import android.os.Bundle;
import android.webkit.PermissionRequest;
import android.webkit.SslErrorHandler;
import android.webkit.WebResourceRequest;
import android.webkit.WebView;
import androidx.activity.OnBackPressedCallback;
import androidx.activity.result.ActivityResultLauncher;
import androidx.activity.result.contract.ActivityResultContracts;
import androidx.core.content.ContextCompat;
import com.getcapacitor.BridgeActivity;
import com.getcapacitor.BridgeWebChromeClient;
import com.getcapacitor.BridgeWebViewClient;
import android.view.View;
import android.view.WindowManager;
import androidx.core.view.ViewCompat;
import androidx.core.view.WindowInsetsAnimationCompat;
import androidx.core.view.WindowInsetsCompat;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

public class MainActivity extends BridgeActivity {
    /** Whether the app is on screen; then notices show inside the interface. */
    static volatile boolean visible;
    static final String NOTICE_NODE = "notice-node", NOTICE_URL = "notice-url";
    private PermissionRequest pendingPermission;
    private boolean mediaPromptOpen;
    private final ActivityResultLauncher<String[]> mediaPermissions = registerForActivityResult(
        new ActivityResultContracts.RequestMultiplePermissions(), this::finishMediaPermission);

    private final ActivityResultLauncher<String> notificationPermission = registerForActivityResult(
        new ActivityResultContracts.RequestPermission(), granted -> { });

    /**
     * Lets the interface of a paired node show a system notification while it is
     * open but without focus. Android's WebView has no Notification API or Web Push.
     */
    private class NoticeBridge {
        @JavascriptInterface
        public void notify(String title, String body, String tag) {
            runOnUiThread(() -> showNotice(title, body, tag));
        }
    }

    private void showNotice(String title, String body, String tag) {
        Uri page = Uri.parse(bridge.getWebView().getUrl() == null ? "" : bridge.getWebView().getUrl());
        int port = page.getPort() == -1 ? 443 : page.getPort();
        // Only the interface of a node this phone paired with may notify.
        if (!"https".equalsIgnoreCase(page.getScheme()) || page.getHost() == null || !Pin.byHost.containsKey(page.getHost() + ":" + port)) return;
        if (Build.VERSION.SDK_INT >= 33 && ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS);
            return;
        }
        if (Build.VERSION.SDK_INT >= 26) {
            ((NotificationManager) getSystemService(NOTIFICATION_SERVICE)).createNotificationChannel(
                new NotificationChannel("notices", "Avisos", NotificationManager.IMPORTANCE_DEFAULT));
        }
        Intent open = new Intent(this, MainActivity.class).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP | Intent.FLAG_ACTIVITY_CLEAR_TOP);
        PendingIntent tap = PendingIntent.getActivity(this, 0, open, PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
        NotificationManagerCompat.from(this).notify(tag == null ? 0 : tag.hashCode(), new NotificationCompat.Builder(this, "notices")
            .setSmallIcon(R.drawable.ic_stat_dory).setContentTitle(title).setContentText(body).setAutoCancel(true).setContentIntent(tap).build());
    }

    @Override
    public void onResume() {
        super.onResume();
        visible = true;
    }

    @Override
    public void onPause() {
        visible = false;
        super.onPause();
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        openNotice(intent);
    }

    /** A tapped notice opens its node, with a fresh session, on the page it points to. */
    private void openNotice(Intent intent) {
        String raw = intent == null ? null : intent.getStringExtra(NOTICE_NODE);
        if (raw == null) return;
        String path = intent.getStringExtra(NOTICE_URL);
        intent.removeExtra(NOTICE_NODE);
        new Thread(() -> {
            try {
                org.json.JSONObject node = new org.json.JSONObject(raw);
                String origin = DoryPlugin.session(node.getString("address"), node.getString("pin"), node.getString("token"));
                runOnUiThread(() -> bridge.getWebView().loadUrl(origin + (path == null || !path.startsWith("/") ? "/" : path)));
            } catch (Exception ignored) {
                // Unreachable node: the app stays on the list of environments.
            }
        }).start();
    }

    private boolean trustedOrigin(Uri origin) {
        if (origin == null || !"https".equalsIgnoreCase(origin.getScheme()) || origin.getHost() == null) return false;
        Uri page = Uri.parse(bridge.getWebView().getUrl() == null ? "" : bridge.getWebView().getUrl());
        int port = origin.getPort() == -1 ? 443 : origin.getPort();
        int pagePort = page.getPort() == -1 ? 443 : page.getPort();
        String pin = Pin.byHost.get(origin.getHost() + ":" + port);
        return "https".equalsIgnoreCase(page.getScheme()) && origin.getHost().equalsIgnoreCase(page.getHost())
            && port == pagePort && pin != null && pin.equals(Pin.of(bridge.getWebView().getCertificate()));
    }

    private void finishMediaPermission(Map<String, Boolean> ignored) {
        mediaPromptOpen = false;
        PermissionRequest request = pendingPermission;
        pendingPermission = null;
        if (request == null) return;
        if (!trustedOrigin(request.getOrigin())) { request.deny(); return; }
        for (String resource : request.getResources()) {
            String permission = resource.equals(PermissionRequest.RESOURCE_AUDIO_CAPTURE) ? Manifest.permission.RECORD_AUDIO : Manifest.permission.CAMERA;
            if (ContextCompat.checkSelfPermission(this, permission) != PackageManager.PERMISSION_GRANTED) { request.deny(); return; }
        }
        request.grant(request.getResources());
    }

    @Override
    public void onCreate(Bundle savedInstanceState) {
        registerPlugin(DoryPlugin.class);
        super.onCreate(savedInstanceState);
        bridge.getWebView().addJavascriptInterface(new NoticeBridge(), "DoryAndroid");
        followKeyboard();
        if (Build.VERSION.SDK_INT >= 33 && ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS);
        }
        NoticeService.start(this);
        openNotice(getIntent());
        bridge.getWebView().setWebChromeClient(new BridgeWebChromeClient(bridge) {
            @Override
            public void onPermissionRequest(PermissionRequest request) {
                if (mediaPromptOpen || !trustedOrigin(request.getOrigin())) { request.deny(); return; }
                ArrayList<String> permissions = new ArrayList<>();
                for (String resource : request.getResources()) {
                    if (resource.equals(PermissionRequest.RESOURCE_AUDIO_CAPTURE)) permissions.add(Manifest.permission.RECORD_AUDIO);
                    else if (resource.equals(PermissionRequest.RESOURCE_VIDEO_CAPTURE)) permissions.add(Manifest.permission.CAMERA);
                    else { request.deny(); return; }
                }
                if (permissions.isEmpty()) { request.deny(); return; }
                pendingPermission = request;
                mediaPromptOpen = true;
                mediaPermissions.launch(permissions.toArray(new String[0]));
            }

            @Override
            public void onPermissionRequestCanceled(PermissionRequest request) {
                if (pendingPermission == request) pendingPermission = null;
            }
        });
        bridge.setWebViewClient(new BridgeWebViewClient(bridge) {
            // A node's self-signed certificate is accepted only if its key is
            // the one pinned when the phone paired with it.
            @Override
            public void onReceivedSslError(WebView view, SslErrorHandler handler, SslError error) {
                Uri u = Uri.parse(error.getUrl());
                String want = Pin.byHost.get(u.getHost() + ":" + u.getPort());
                if (want != null && want.equals(Pin.of(error.getCertificate()))) handler.proceed();
                else handler.cancel();
            }

            // Links inside an open node stay in the app.
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                Uri u = request.getUrl();
                if (Pin.byHost.containsKey(u.getHost() + ":" + u.getPort())) return false;
                return super.shouldOverrideUrlLoading(view, request);
            }
        });
        // Back on the first page of a node returns to the list of environments.
        getOnBackPressedDispatcher().addCallback(this, new OnBackPressedCallback(true) {
            @Override
            public void handleOnBackPressed() {
                WebView w = bridge.getWebView();
                if (w.canGoBack()) w.goBack();
                else if (!bridge.getLocalUrl().equals(w.getUrl()) && !w.getUrl().startsWith(bridge.getLocalUrl())) w.loadUrl(bridge.getLocalUrl());
                else finish();
            }
        });
    }

    private int lastIme = -1;

    // followKeyboard lets the keyboard overlay the WebView instead of resizing
    // it, and passes the page the height it covers on every frame of its
    // animation; the page's screens shrink from the bottom with it.
    private void followKeyboard() {
        getWindow().setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_NOTHING);
        View decor = getWindow().getDecorView();
        ViewCompat.setWindowInsetsAnimationCallback(decor, new WindowInsetsAnimationCompat.Callback(WindowInsetsAnimationCompat.Callback.DISPATCH_MODE_CONTINUE_ON_SUBTREE) {
            @Override
            public WindowInsetsCompat onProgress(WindowInsetsCompat insets, List<WindowInsetsAnimationCompat> running) {
                sendIme(imeHeight(decor, insets));
                return insets;
            }

            @Override
            public void onEnd(WindowInsetsAnimationCompat animation) {
                WindowInsetsCompat insets = ViewCompat.getRootWindowInsets(decor);
                if (insets != null) sendIme(imeHeight(decor, insets));
            }
        });
    }

    private void sendIme(int css) {
        if (css == lastIme) return;
        lastIme = css;
        bridge.getWebView().evaluateJavascript("window.__doryKeyboard&&__doryKeyboard(" + css + ")", null);
    }

    // imeHeight is how much of the WebView the keyboard covers, in CSS pixels.
    private int imeHeight(View decor, WindowInsetsCompat insets) {
        // The decor view is already padded above the navigation bar.
        int px = Math.max(0, insets.getInsets(WindowInsetsCompat.Type.ime()).bottom - decor.getPaddingBottom());
        return Math.round(px / getResources().getDisplayMetrics().density);
    }
}
