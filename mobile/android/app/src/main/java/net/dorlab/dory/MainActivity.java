package net.dorlab.dory;

import android.net.Uri;
import android.net.http.SslError;
import android.os.Bundle;
import android.webkit.SslErrorHandler;
import android.webkit.WebResourceRequest;
import android.webkit.WebView;
import androidx.activity.OnBackPressedCallback;
import com.getcapacitor.BridgeActivity;
import com.getcapacitor.BridgeWebViewClient;

public class MainActivity extends BridgeActivity {
    @Override
    public void onCreate(Bundle savedInstanceState) {
        registerPlugin(DoryPlugin.class);
        super.onCreate(savedInstanceState);
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
}
