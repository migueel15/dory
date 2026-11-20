package net.dorlab.dory;

import android.content.Context;
import android.content.SharedPreferences;
import javax.net.ssl.HttpsURLConnection;
import org.json.JSONArray;
import org.json.JSONObject;

/** Private copy of the launcher settings for assistant invocations without a WebView. */
final class CaptureNodes {
    private static SharedPreferences settings(Context context) {
        return context.getSharedPreferences("idea-capture", Context.MODE_PRIVATE);
    }

    static boolean save(Context context, JSONArray nodes, String destination) {
        return settings(context).edit()
            .putString("nodes", nodes.toString())
            .putString("destination", destination)
            .commit();
    }

    static JSONObject destination(Context context, String retryNode) throws Exception {
        SharedPreferences settings = settings(context);
        JSONArray nodes = new JSONArray(settings.getString("nodes", "[]"));
        if (nodes.length() == 0) {
            throw new Exception("Abre Dory y comprueba que hay un nodo vinculado.");
        }
        if (retryNode == null && nodes.length() == 1) return nodes.getJSONObject(0);
        String id = retryNode == null ? settings.getString("destination", "") : retryNode;
        for (int i = 0; i < nodes.length(); i++) {
            JSONObject node = nodes.getJSONObject(i);
            if (!id.isEmpty() && id.equals(node.optString("id"))) return node;
        }
        throw new Exception(retryNode == null ? "Elige el nodo de captura en Dory." : "El nodo de esta idea ya no está vinculado.");
    }

    static void sendIdea(JSONObject node, String id, String text) throws Exception {
        String pin = node.getString("pin");
        String token = node.getString("token");
        if (pin.isEmpty() || token.isEmpty()) throw new Exception("Vuelve a emparejar el nodo de captura.");
        JSONObject body = new JSONObject().put("id", id).put("text", text);
        HttpsURLConnection connection = Pin.open("https://" + node.getString("address") + "/api/ideas", pin, null);
        connection.setInstanceFollowRedirects(false);
        try {
            String reply = DoryPlugin.send(connection, "POST", token, body.toString());
            if (!id.equals(new JSONObject(reply).optString("id"))) {
                throw new Exception("El nodo no confirmó el guardado.");
            }
        } finally {
            connection.disconnect();
        }
    }
}
