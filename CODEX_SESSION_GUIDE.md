# Guide: Régénérer le Session Cookie Codex/Defined.fi

**À faire tous les 7 jours quand le session cookie expire**

## Symptômes d'expiration

Sur Railway, tu verras dans les logs:
```
[CODEX-REST] Failed to get JWT token: unexpected status 401
[HEAD-LAG][CODEX] Connection error: failed to get JWT token: unexpected status 401
```

## Étapes pour régénérer

### 1. Générer un nouveau session cookie (en local)

```bash
cd cmd/script
go run session_scraper.go
```

**Attendre 5-10 secondes**, le script va:
- Ouvrir Chrome en mode headless
- Visiter https://www.defined.fi/
- Récupérer le session cookie
- Afficher le résultat

**Output attendu:**
```
[SESSION-SCRAPER] Found session cookie (length: 147)
[SESSION-SCRAPER] ✓ Session cookie refreshed successfully (length: 147)
```

Le cookie est de la forme: `eyJhbGciOiJIUzI1NiJ9.eyJleHBpcmVzQXQi...`

### 2. Copier le cookie

Le cookie est affiché dans la console. Copie-le entièrement (147 caractères).

### 3. Update sur Railway

**Option A - Via UI:**
1. Va sur Railway dashboard
2. Sélectionne ton service
3. Variables → `DEFINED_SESSION_COOKIE`
4. Paste le nouveau cookie
5. Redeploy

**Option B - Via CLI:**
```bash
railway variables --set DEFINED_SESSION_COOKIE="eyJhbGciOiJIUzI1NiJ9.ey..."
railway up --detach
```

### 4. Vérifier que ça marche

Dans les logs Railway, tu dois voir:
```
Using DEFINED_SESSION_COOKIE from environment (length: 147)
[DEFINED-AUTH] JWT token refreshed. Expires in 24.0h
[CODEX-REST][solana] ✓ | Latency: 217ms | Status: 200
[HEAD-LAG][CODEX] Subscribed to 5 pools
```

## Combien de temps ça dure?

- **Session Cookie:** 7 jours
- **JWT Token:** 24h (auto-renouvelé par le code tant que session cookie valide)

Donc **regénérer tous les 7 jours**.

## Problèmes courants

### "429 Rate Limited" quand je teste

**Cause:** Trop de tentatives de génération JWT en peu de temps.

**Solution:** Attendre 1-2 heures, le rate limit va expirer.

**Workaround:** Utiliser un VPN ou changer d'IP.

### "Session cookie not found"

**Cause:** Problème avec Chrome headless (Cloudflare bloque).

**Solution:**
1. Essayer 2-3 fois
2. Si ça persiste, le site a peut-être changé leur protection

### "401 Unauthorized" même avec nouveau cookie

**Cause:** Le cookie est invalide ou mal copié.

**Solution:**
1. Vérifier que tu as copié le cookie EN ENTIER (147 caractères)
2. Pas d'espaces avant/après
3. Regénérer un nouveau

## Test en local avant Railway

Avant d'update Railway, teste que le cookie marche:

```bash
# 1. Update .env local
echo "DEFINED_SESSION_COOKIE=ton_nouveau_cookie" > .env

# 2. Build et run
make build
./bin/monitor

# 3. Check les logs
# Tu dois voir: [CODEX-REST] ✓ | Latency: ...
```

Si ça marche en local → update Railway.

## Notes techniques

### Pourquoi pas automatiser sur Railway?

Le scraping nécessite Chrome/Chromium, difficile à run sur Railway (pas de GUI).

Alternatives explorées:
- ✗ Session cookie seul → 4403 Forbidden sur WebSocket
- ✗ Bypass JWT → Defined.fi a fermé cette méthode
- ✓ **Solution actuelle:** Session cookie → JWT (marche)

### Structure du flow

```
Session Cookie (7 jours)
    ↓
JWT Token (24h, auto-renouvelé)
    ↓
Codex WebSocket (temps réel)
```

Le code gère automatiquement:
- Cache du JWT
- Auto-renouvellement 1h avant expiration
- Rate limit handling (skip gracefully)
- Retry logic

Tu dois juste regénérer le session cookie tous les 7 jours.

## Script de renouvellement rapide

Crée `renew_codex.sh` en local:

```bash
#!/bin/bash
echo "🔄 Generating new Codex session cookie..."
cd cmd/script
NEW_COOKIE=$(go run session_scraper.go 2>&1 | grep "Session cookie:" | cut -d':' -f2 | xargs)

if [ -z "$NEW_COOKIE" ]; then
    echo "❌ Failed to generate cookie"
    exit 1
fi

echo "✅ New cookie: ${NEW_COOKIE:0:50}..."
echo ""
echo "📋 Update on Railway:"
echo "railway variables --set DEFINED_SESSION_COOKIE=\"$NEW_COOKIE\""
echo ""
echo "Or copy this:"
echo "$NEW_COOKIE"
```

Usage:
```bash
chmod +x renew_codex.sh
./renew_codex.sh
```

## Monitoring

Pour savoir quand renouveler, check les logs Railway tous les 6-7 jours.

Ou setup une alerte sur l'erreur `401` dans les logs.
