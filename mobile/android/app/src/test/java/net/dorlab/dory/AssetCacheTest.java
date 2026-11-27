package net.dorlab.dory;

import static org.junit.Assert.assertArrayEquals;
import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertFalse;
import static org.junit.Assert.assertNotNull;
import static org.junit.Assert.assertNull;
import static org.junit.Assert.assertTrue;

import java.io.ByteArrayInputStream;
import java.io.File;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import org.junit.Rule;
import org.junit.Test;
import org.junit.rules.TemporaryFolder;

public class AssetCacheTest {
    @Rule
    public TemporaryFolder tmp = new TemporaryFolder();

    @Test
    public void assetNameFiltersValidAndRejectsInvalidPaths() {
        assertEquals("index-Ab12_3.js", AssetCache.assetName("/assets/index-Ab12_3.js"));
        assertEquals("style.css", AssetCache.assetName("/assets/style.css"));
        assertNull(AssetCache.assetName(null));
        assertNull(AssetCache.assetName("/"));
        assertNull(AssetCache.assetName("/api/node"));
        assertNull(AssetCache.assetName("/assets/"));
        assertNull(AssetCache.assetName("/assets/../secret.js"));
        assertNull(AssetCache.assetName("/assets/sub/file.js"));
        assertNull(AssetCache.assetName("/assets/a..b.js"));
        assertNull(AssetCache.assetName("/assets/bad%20name.js"));
    }

    @Test
    public void mimeTypeAndCharsetDetection() {
        assertEquals("application/javascript", AssetCache.mimeType("app-123.js"));
        assertEquals("application/javascript", AssetCache.mimeType("mod.mjs"));
        assertEquals("text/css", AssetCache.mimeType("main.css"));
        assertEquals("font/woff2", AssetCache.mimeType("font.woff2"));
        assertEquals("font/woff", AssetCache.mimeType("font.woff"));
        assertEquals("font/ttf", AssetCache.mimeType("font.ttf"));
        assertEquals("image/svg+xml", AssetCache.mimeType("icon.svg"));
        assertEquals("image/png", AssetCache.mimeType("logo.png"));
        assertEquals("application/wasm", AssetCache.mimeType("core.wasm"));
        assertEquals("application/octet-stream", AssetCache.mimeType("data.bin"));

        assertEquals("UTF-8", AssetCache.charset("application/javascript"));
        assertEquals("UTF-8", AssetCache.charset("text/css"));
        assertEquals("UTF-8", AssetCache.charset("image/svg+xml"));
        assertNull(AssetCache.charset("font/woff2"));
        assertNull(AssetCache.charset("image/png"));
        assertNull(AssetCache.charset("application/octet-stream"));
    }

    @Test
    public void putAndGetReadsCachedBytes() throws Exception {
        File dir = tmp.newFolder("web-assets");
        AssetCache cache = new AssetCache(dir, 1024);
        assertNull(cache.get("missing.js"));

        byte[] content = "console.log('hi');".getBytes(StandardCharsets.UTF_8);
        File saved = cache.put("app-1.js", new ByteArrayInputStream(content));
        assertTrue(saved.isFile());

        File hit = cache.get("app-1.js");
        assertNotNull(hit);
        assertArrayEquals(content, Files.readAllBytes(hit.toPath()));
    }

    @Test
    public void pruneEvictsOldestFilesFirst() throws Exception {
        File dir = tmp.newFolder("web-assets-lru");
        AssetCache cache = new AssetCache(dir, 20);

        File f1 = cache.put("a.js", new ByteArrayInputStream(new byte[10]));
        assertTrue(f1.setLastModified(1_000L));
        File f2 = cache.put("b.js", new ByteArrayInputStream(new byte[10]));
        assertTrue(f2.setLastModified(2_000L));

        // Total is 20 (at limit). Adding 10 more should evict oldest (a.js).
        File f3 = cache.put("c.js", new ByteArrayInputStream(new byte[10]));
        assertTrue(f3.isFile());
        assertFalse(f1.exists());
        assertTrue(f2.exists());
    }
}
