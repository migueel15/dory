package net.dorlab.dory;

import android.net.http.SslCertificate;
import android.os.Bundle;
import android.util.Base64;
import java.io.ByteArrayInputStream;
import java.security.MessageDigest;
import java.security.cert.CertificateException;
import java.security.cert.CertificateFactory;
import java.security.cert.X509Certificate;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import javax.net.ssl.HttpsURLConnection;
import javax.net.ssl.SSLContext;
import javax.net.ssl.SSLSocketFactory;
import javax.net.ssl.TrustManager;
import javax.net.ssl.X509TrustManager;

/**
 * Nodes use self-signed certificates. Trust comes from pairing: the app
 * pins the SHA-256 of the public key of each node, per host.
 */
final class Pin {
    /** host:port → pin of the node the app is talking to. */
    static final Map<String, String> byHost = new ConcurrentHashMap<>();
    /** pin → reusable SSLSocketFactory for TLS session resumption and keep-alive. */
    private static final Map<String, SSLSocketFactory> factories = new ConcurrentHashMap<>();

    static String of(X509Certificate cert) throws Exception {
        byte[] sum = MessageDigest.getInstance("SHA-256").digest(cert.getPublicKey().getEncoded());
        return Base64.encodeToString(sum, Base64.URL_SAFE | Base64.NO_PADDING | Base64.NO_WRAP);
    }

    /** Pin of a certificate the web view reports in an SSL error. */
    static String of(SslCertificate cert) {
        try {
            Bundle b = SslCertificate.saveState(cert);
            byte[] der = b.getByteArray("x509-certificate");
            if (der == null) return null;
            X509Certificate x = (X509Certificate) CertificateFactory.getInstance("X.509").generateCertificate(new ByteArrayInputStream(der));
            return of(x);
        } catch (Exception e) {
            return null;
        }
    }

    /**
     * Opens a connection that accepts only the given pin. With want == null
     * it accepts any key and stores it in seen[0]: used only to pair, where
     * the code proves who answered.
     */
    static HttpsURLConnection open(String url, String want, String[] seen) throws Exception {
        HttpsURLConnection c = (HttpsURLConnection) new java.net.URL(url).openConnection();
        SSLSocketFactory sf;
        if (want != null && seen == null) {
            sf = factories.get(want);
            if (sf == null) {
                sf = createSocketFactory(want, null);
                factories.put(want, sf);
            }
        } else {
            sf = createSocketFactory(want, seen);
        }
        c.setSSLSocketFactory(sf);
        c.setHostnameVerifier((h, s) -> true); // the pin, not the name, identifies the node
        c.setConnectTimeout(4000);
        c.setReadTimeout(15000);
        return c;
    }

    private static SSLSocketFactory createSocketFactory(String want, String[] seen) throws Exception {
        X509TrustManager tm = new X509TrustManager() {
            public void checkClientTrusted(X509Certificate[] chain, String type) {}
            public void checkServerTrusted(X509Certificate[] chain, String type) throws CertificateException {
                try {
                    String got = of(chain[0]);
                    if (seen != null) seen[0] = got;
                    if (want != null && !want.equals(got)) throw new CertificateException("la clave del nodo no es la emparejada");
                } catch (CertificateException e) {
                    throw e;
                } catch (Exception e) {
                    throw new CertificateException(e);
                }
            }
            public X509Certificate[] getAcceptedIssuers() { return new X509Certificate[0]; }
        };
        SSLContext ctx = SSLContext.getInstance("TLS");
        ctx.init(null, new TrustManager[] { tm }, null);
        return ctx.getSocketFactory();
    }
}
