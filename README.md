# Claude Desktop Gateway

Go CLI that configures Claude Desktop (3P gateway mode) and runs a local
Anthropic Messages API proxy that routes to OpenRouter, 9router, or any
OpenAI-compatible gateway.

**You only need the binary + an OpenRouter API key.** No repo checkout and no
hand-written config are required: on first run the binary writes the shipped
default `config.toml` to `~/.config/claude-gateway/config.toml`.

---

## Install (binary from GitHub Releases)

Every push to `main` builds multi-platform binaries and publishes / updates a
rolling GitHub Release tagged **`latest`**.

### Recommended (works while the repo is private)

```bash
# requires: gh auth login   (access to Danakolana/claude-gateway)
gh release download latest -R Danakolana/claude-gateway \
  -p 'claude-gateway-linux-amd64' -O claude-gateway
chmod +x claude-gateway

export OPENROUTER_API_KEY=sk-or-...
./claude-gateway
```

Pick another asset if needed:

| File | Platform |
|---|---|
| `claude-gateway-linux-amd64` | Linux x86_64 |
| `claude-gateway-linux-arm64` | Linux ARM64 |
| `claude-gateway-darwin-amd64` | macOS Intel |
| `claude-gateway-darwin-arm64` | macOS Apple Silicon |
| `claude-gateway-windows-amd64.exe` | Windows x86_64 |

### Public `curl` (only after the repo — or a releases mirror — is public)

```bash
curl -fsSL -o claude-gateway \
  https://github.com/Danakolana/claude-gateway/releases/latest/download/claude-gateway-linux-amd64
chmod +x claude-gateway
export OPENROUTER_API_KEY=sk-or-...
./claude-gateway
```

### Can a private repo have a public release binary?

**No — not on the same private GitHub repo.** Release assets inherit repository
visibility. Anonymous `curl` to `/releases/latest/download/...` returns 404
while the repo is private.

Practical options:

| Approach | Public download? | Notes |
|---|---|---|
| Keep repo private; users use `gh` / PAT | No (auth required) | Best for a small trusted circle |
| Make the repo public | Yes | Simplest public `curl` |
| Separate **public** repo (e.g. `claude-gateway-binaries`) that only hosts Releases | Yes | Source stays private; CI uploads artifacts there with a deploy key / `GH_TOKEN` |
| Public object storage (R2 / S3 / Cloudflare) | Yes | CI uploads after build |
| Gist | Poor fit | Size/rate limits; awkward for multi-arch binaries |

Gist is **not** a good home for these binaries. Prefer a public releases-only
repo or object storage if you want anonymous `curl` while keeping source private.

---

## First run / config

Priority when resolving config:

1. `--config PATH`
2. `CLAUDE_GATEWAY_CONFIG`
3. `~/.config/claude-gateway/config.toml`
4. `~/.claude-gateway/config.toml`
5. `./config.toml` (repo checkout)
6. *(legacy)* `./examples/config.toml`

If none exist, the embedded default (same as root [`config.toml`](./config.toml)
from the build commit) is written to `~/.config/claude-gateway/config.toml`.
Existing user files are never overwritten.

```bash
export OPENROUTER_API_KEY=sk-or-...
./claude-gateway
# → First run: wrote default config to ~/.config/claude-gateway/config.toml
```

Listen address and Desktop apply come from:

```toml
[proxy]
mode = "local"          # or "direct" = Desktop → OpenRouter (no local proxy)
listen = "127.0.0.1:8080"
apply_desktop = true
```

### Build from source

```bash
make build
# binaries: dist/claude-gateway  dist/gateway-server
make check   # tests + staticcheck + docscheck
./dist/claude-gateway
```

---

## Quick start notes

**Compare without local proxy** (OpenRouter’s Anthropic-compatible API):

```toml
[proxy]
mode = "direct"
```

Then run `./claude-gateway` — it writes Desktop **Connection** settings to
OpenRouter for **Anthropic-looking IDs only** (`anthropic/claude-*`). Remapped
budget models need `mode = "local"`. Quit/reopen Desktop afterward.

Optional flags: `--config PATH`, `--listen HOST:PORT`, `--no-apply`, `--fake`.

### Which Claude Desktop? (interactive)

On start (when applying), the CLI asks:

1. **3P (recommended)** — Connection Gateway UI + custom model labels
2. **Consumer (experimental)** — regular Desktop via `env.ANTHROPIC_BASE_URL` only

Non-interactive runs (no TTY) default to **3P**.

### OpenRouter mirror / reverse proxy

```toml
[providers.openrouter]
base_url = "https://your-mirror.example.com/api/v1"
# allow_private_network = true  # only for private LAN mirrors
```

Or: `export OPENROUTER_BASE_URL=https://your-mirror.example.com/api/v1`

---

## Default models (Desktop dropdown)

Root [`config.toml`](./config.toml) (also embedded in the binary) includes
budget remaps **and** official Claude models via OpenRouter. Desktop only
accepts Anthropic-looking IDs — the gateway maps `desktop_id` → real `model_id`.

Approximate OpenRouter prices (USD per 1M tokens). Snapshot dated **2026-09-11**
— not a billing guarantee; re-check with `./claude-gateway models status`.

| Desktop ID | Upstream | In $/M | Out $/M | Notes |
|---|---|---:|---:|---|
| `claude-haiku-4` | `deepseek/deepseek-v4-flash-0731` | 0.065 | 0.18 | DeepSeek V4 Flash (default haiku) |
| `claude-haiku-4-1` | `z-ai/glm-5.3-flash` | 0.15 | 0.50 | GLM 5.3 Flash |
| `claude-haiku-4-2` | `inception/mercury-2.5-preview` | 0.04* | 0.15* | Mercury 2.5 (*config; may miss live catalog) |
| `claude-haiku-4-3` | `minimax/minimax-m2.5` | 0.30 | 1.20 | MiniMax M2.5 |
| `claude-haiku-4-4` | `qwen/qwen3-coder-next` | 0.12 | 0.80 | Qwen3 Coder Next |
| `claude-haiku-4-6` | `deepseek/deepseek-v4.1-flash` | 0.15 | 0.60 | DeepSeek V4.1 Flash |
| `claude-haiku-4-7` | `qwen/qwen3.8-flash` | 0.15 | 0.47 | Qwen3.8 Flash |
| `claude-haiku-4-8` | `google/gemini-3.8-flash` | 0.75 | 3.75 | Gemini 3.8 Flash |
| `claude-haiku-4-9` | `ibm-granite/granite-4.2-8b` | 0.06 | 0.25 | Granite 4.2 8B |
| `claude-haiku-4-10` | `nvidia/nemotron-3.5-lightning` | 0.08 | 0.20 | Nemotron 3.5 Lightning |
| `claude-sonnet-4` | `moonshotai/kimi-k2.5` | 0.45 | 2.25 | Kimi K2.5 (default sonnet) |
| `claude-sonnet-4-1` | `moonshotai/kimi-k2.6` | 0.95 | 4.00 | Kimi K2.6 |
| `claude-sonnet-4-2` | `moonshotai/kimi-k2.7-code` | 0.71 | 3.50 | Kimi K2.7 Code |
| `claude-sonnet-4-3` | `deepseek/deepseek-v3.2` | 0.269 | 0.40 | DeepSeek V3.2 |
| `claude-sonnet-4-4` | `meta/muse-spark-1.3` | 1.25 | 4.25 | Muse Spark 1.3 |
| `claude-sonnet-4-6` | `meta/muse-spark-1.2` | 1.25 | 4.25 | Muse Spark 1.2 |
| `claude-sonnet-4-7` | `openai/gpt-5.6-luna` | 0.20 | 1.20 | GPT-5.6 Luna |
| `claude-sonnet-4-8` | `tencent/hy4-preview` | 0.834 | 2.501 | Hy4 Preview |
| `claude-opus-4-1` | `x-ai/grok-4.6` | 2.00 | 6.00 | Grok 4.6 |
| `claude-opus-4-2` | `x-ai/grok-4.5` | 2.00 | 6.00 | Grok 4.5 |
| `anthropic/claude-sonnet-4` | same | 3.00 | 15.00 | Official Claude Sonnet 4 |
| `anthropic/claude-haiku-4.5` | same | 1.00 | 5.00 | Official Claude Haiku 4.5 |
| `anthropic/claude-3-haiku` | same | 0.25 | 1.25 | Official Claude Haiku 3 |
| `anthropic/claude-sonnet-4.5` | same | 3.00 | 15.00 | Official Claude Sonnet 4.5 |
| `anthropic/claude-sonnet-4.6` | same | 3.00 | 15.00 | Official Claude Sonnet 4.6 |
| `anthropic/claude-sonnet-5` | same | 2.00 | 10.00 | Official Claude Sonnet 5 |
| `anthropic/claude-opus-4.6` | same | 5.00 | 25.00 | Official Claude Opus 4.6 |

```bash
./claude-gateway models list
./claude-gateway models status
./claude-gateway models status --watch 60
```

Add models under `[models.*]` in your local config, then re-run so Desktop
picks them up (`apply_desktop = true` on start, or `client apply`).

---

## Cost & token burn

Full plan: [`docs/COST.md`](docs/COST.md).

- Remap Desktop models → cheaper OpenRouter IDs (main savings)
- `routing.prefer_cheapest`, `routing.ensure_prompt_cache`
- `models.*.thinking_policy` (`force_off` / `cap` / `passthrough`)
- `providers.*.sort = "price"`

## Optional history sync server

```bash
export CLAUDE_GATEWAY_SYNC_TOKEN=long-random-token
./gateway-server --addr 127.0.0.1:8090 --data ~/.local/share/claude-gateway/server
```

## Safety

- Does not patch Claude Desktop binaries or bypass Anthropic auth.
- Uses documented Claude Desktop **on 3P** gateway settings (ADR-011).
- Proxy binds to loopback by default; secrets stay in env vars, not TOML.

---

## راهنمای فارسی (مفصل)

### این پروژه چیست؟

**Claude Desktop Gateway** یک برنامه‌ی خط‌فرمان (CLI) به زبان Go است که دو کار اصلی می‌کند:

1. **پیکربندی Claude Desktop** در حالت درگاه شخص‌ثالث (3P / developer) تا مدل‌های دلخواه شما در لیست مدل‌ها دیده شوند.
2. **اجرای یک پروکسی محلی** با API سازگار با Anthropic Messages (`POST /v1/messages`) که درخواست‌های Desktop را به **OpenRouter** (یا آینه‌ی سازگار / 9router) هدایت می‌کند.

نتیجه: در Claude Desktop مدل‌هایی مثل DeepSeek، Kimi، GLM، Qwen و … را با برچسب‌های خوانا می‌بینید، در حالی که از نظر Desktop شناسه‌ها شبیه `claude-*` یا `anthropic/claude-*` هستند و gateway آن‌ها را به مدل واقعی OpenRouter نگاشت می‌کند.

### نصب فقط با باینری (بدون کلون کردن ریپو)

هدف نهایی این است که کاربر **نیازی به داشتن `config.toml` از قبل** نداشته باشد.

- با هر push به شاخه‌ی `main`، GitHub Actions باینری را برای لینوکس / مک / ویندوز می‌سازد و در Release با تگ **`latest`** منتشر می‌کند.
- داخل باینری، آخرین `config.toml` همان کامیت **جاسازی (embed)** شده است.
- در **اولین اجرا**، اگر در سیستم شما فایل کانفیگ پیدا نشود، همان کانفیگ پیش‌فرض در مسیر زیر نوشته می‌شود:

```text
~/.config/claude-gateway/config.toml
```

اگر این فایل از قبل وجود داشته باشد، **دست‌نخورده می‌ماند** (بازنویسی نمی‌شود).

#### نصب پیشنهادی (ریپو فعلاً خصوصی است)

```bash
gh auth login   # یک‌بار
gh release download latest -R Danakolana/claude-gateway \
  -p 'claude-gateway-linux-amd64' -O claude-gateway
chmod +x claude-gateway

export OPENROUTER_API_KEY=sk-or-...
./claude-gateway
```

پیام شبیه این را باید ببینید:

```text
First run: wrote default config to /home/YOU/.config/claude-gateway/config.toml
```

بعد از آن Claude Desktop را باز کنید و در صورت نیاز **Apply Changes** را بزنید.

#### آیا می‌شود ریپو خصوصی بماند ولی فایل Release عمومی باشد؟

**خیر — روی همان ریپوی خصوصی GitHub.** دارایی‌های Release همان سطح دسترسی ریپو را دارند؛ لینک `curl` بدون لاگین برای دیگران کار نمی‌کند.

راه‌حل‌های واقعی اگر `curl` عمومی می‌خواهید:

1. **ریپوی عمومی جدا فقط برای باینری** (مثلاً `claude-gateway-binaries`) — سورس خصوصی می‌ماند؛ CI آرتیفکت را آنجا آپلود می‌کند.
2. **ذخیره‌سازی آبجکت عمومی** (Cloudflare R2، S3، …).
3. **عمومی کردن خود ریپو**.

Gist برای باینری چند معماری گزینه‌ی مناسبی نیست (محدودیت حجم و مدیریت سخت).

وقتی Release عمومی شد، نصب ساده این است:

```bash
curl -fsSL -o claude-gateway \
  https://github.com/Danakolana/claude-gateway/releases/latest/download/claude-gateway-linux-amd64
chmod +x claude-gateway
```

### ترتیب پیدا کردن کانفیگ

1. فلگ `--config مسیر/فایل.toml`
2. متغیر محیطی `CLAUDE_GATEWAY_CONFIG`
3. `~/.config/claude-gateway/config.toml` ← مسیر پیش‌فرض کاربر
4. `~/.claude-gateway/config.toml`
5. `./config.toml` در پوشه‌ی جاری (مثلاً وقتی از سورس کار می‌کنید)
6. مسیر قدیمی `./examples/config.toml` (فقط سازگاری عقب‌رو)

کانفیگ رسمی پروژه در ریشه ریپو است: [`config.toml`](./config.toml) — دیگر وابستگی اجباری به پوشه‌ی `examples/` ندارید.

### بعد از نصب چه کار کنید؟

1. کلید OpenRouter را تنظیم کنید: `export OPENROUTER_API_KEY=sk-or-...`
2. باینری را اجرا کنید: `./claude-gateway`
3. اگر از شما پرسید Desktop را کجا تنظیم کند، گزینهٔ **1) 3P** را انتخاب کنید (پیشنهادی).
4. Claude Desktop را ری‌استارت کنید (یا Apply Changes).
5. در انتخاب مدل، برچسب‌هایی مثل DeepSeek / Kimi / … را ببینید.

دستورهای مفید:

```bash
./claude-gateway models list     # نگاشت Desktop → OpenRouter
./claude-gateway models status   # قیمت و کانتکست زنده از OpenRouter
```

### حالت‌های پروکسی

| مقدار `[proxy] mode` | معنی |
|---|---|
| `local` (پیش‌فرض) | Desktop → پروکسی محلی → OpenRouter — مناسب remap مدل‌های ارزان |
| `direct` | Desktop مستقیم به API سازگار Anthropicِ OpenRouter — فقط مدل‌های `anthropic/claude-*` |

### ویرایش مدل‌ها

فایل `~/.config/claude-gateway/config.toml` را باز کنید، در انتهای فایل بلوک `[models....]` و قانون routing مربوطه را اضافه/ویرایش کنید، سپس دوباره `./claude-gateway` را اجرا کنید تا تنظیمات Desktop به‌روز شود.

`desktop_id` باید شبیه شناسه‌های Anthropic باشد، مثلاً `claude-haiku-4-3` یا `anthropic/claude-sonnet-4`.

### امنیت و حریم خصوصی

- کلید API را داخل TOML نگذارید؛ از متغیر محیطی استفاده کنید.
- پروکسی پیش‌فرض فقط روی `127.0.0.1` گوش می‌دهد.
- این ابزار باینری Claude Desktop را پچ نمی‌کند و احراز هویت Anthropic را دور نمی‌زند.

### ساخت از سورس (برای توسعه‌دهنده)

```bash
make build    # از config.toml کپی به embed + بیلد
make check
./dist/claude-gateway
```

هر بار که `config.toml` ریشه را عوض می‌کنید، قبل از کامیت `make sync-default-config` (یا `make build`) را بزنید تا نسخه‌ی embed‌شده هم‌خوان بماند؛ تست `TestEmbeddedDefaultMatchesRoot` این را چک می‌کند.
