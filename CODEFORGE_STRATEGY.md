# CodeForge TUI — Strategi Transformasi Menuju Produk Mutakhir
### Dari "Alpha Fungsional" menjadi Terminal AI Companion Kelas Dunia

> **Codename internal:** `NEO-FORGE`
> **Disusun untuk:** NanoMind — Author & Maintainer CodeForge
> **Status saat ini:** `v0.3.0` · Neo-Forge Fase 0–5 diimplementasikan (lihat checklist §7)
> **Target:** Visual mutakhir, alur kerja modern, arsitektur yang siap berkembang tanpa ditulis ulang

---

## 0. Cara Membaca Dokumen Ini

Dokumen ini bukan wishlist generik. Setiap temuan di Bagian 1 diverifikasi langsung dari kode sumber `NanoMindExplorer/codeforge` (branch `main`, 15 file `.go`, ~ 1.100 baris). Setiap rekomendasi di Bagian 3–8 dirancang agar bisa langsung dieksekusi dengan alur kerja yang sudah Nano gunakan: `grep` string unik → buat patch → `git apply` → push → CI build → pasang binary → uji di device. Tidak ada langkah yang mengasumsikan penulisan ulang dari nol — CodeForge sudah punya fondasi arsitektur yang bersih (pemisahan `agent` / `provider` / `tool` / `tui` sangat rapi), jadi strategi ini adalah **augmentasi terarah**, bukan bongkar total.

Struktur dokumen:

1. Audit jujur kondisi saat ini (kelebihan & utang teknis nyata)
2. Visi & filosofi desain — "Terminal Glass / Neo-Forge Aesthetic"
3. Design system lengkap (warna, tipografi, ikon, motion, elevation)
4. Arsitektur software baru untuk menopang desain ini
5. Redesign per komponen (before → after, dengan detail implementasi)
6. Alur kerja produk baru (fitur-fitur yang mengubah CodeForge dari "chatbot di terminal" menjadi "agentic coding companion")
7. Peta jalan fase-demi-fase, siap dieksekusi sebagai patch series
8. Standar kualitas, testing, dan kontrak kerja AI (CFG-AEC)
9. Strategi rilis & distribusi
10. Definisi "selesai" — metrik sukses konkret

---

## 1. Audit Kondisi Saat Ini

### 1.1 Yang sudah kuat (jangan disentuh, tinggal dibungkus ulang)

| Area | Kondisi | Kenapa ini aset, bukan beban |
|---|---|---|
| Pemisahan modul | `internal/agent`, `internal/provider`, `internal/tool`, `internal/git`, `internal/tui` benar-benar terpisah | `tui` tidak pernah tahu detail provider — tinggal ganti *rendering*, logic aman |
| Event-driven agent loop | `agent.Run()` mengembalikan `<-chan agent.Event` (`EventText`, `EventToolCall`, `EventToolResult`, `EventDone`, `EventError`) | Arsitektur ini persis yang dibutuhkan animasi timeline — tidak perlu diubah, tinggal dikonsumsi lebih kaya di UI |
| Streaming yang benar | Bug `context.WithTimeout` + `defer cancel()` yang memutus stream sudah diperbaiki jadi `context.Background()` | Fondasi real-time UI sudah solid |
| Tool sandbox | `resolvePath()` menolak path yang keluar dari `workdir` | Aman untuk fitur `@file` mention nanti |
| Single binary ~11MB | Cocok Termux/Android | Tidak boleh hilang — semua dependency baru harus tetap ringan |

### 1.2 Utang teknis nyata (ditemukan dari pembacaan kode langsung)

Ini bukan dugaan — ini bug/gap konkret yang **harus** dibereskan di Fase 0, sebelum reskin visual dimulai. Kalau dibiarkan, reskin akan membungkus bug, bukan menyelesaikannya (melanggar prinsip *root cause before fix* yang sudah Nano tetapkan sendiri di kontrak GMM-AEC).

1. **`ContextUpdateMsg` tidak pernah dikirim.** `internal/tui/context.go` punya `case ContextUpdateMsg:` yang siap menerima daftar file, tapi tidak ada satupun pemanggil di `model.go` atau `chat.go` yang mengirim pesan ini. Context pane secara fungsional **mati** — sama persis pola "dead overlay toggle button" yang pernah ditemukan di GameMapperMind.
2. **`/model <name>` adalah stub.** Di `model.go`, cabang `case "model", "m":` hanya menampilkan teks `"Model akan digunakan: " + argStr` tanpa benar-benar memanggil registry untuk berpindah model. Ini melanggar aturan "no stubs" di kontrak kerja Nano sendiri.
3. **`write_file` mengeksekusi tanpa gerbang konfirmasi.** Tool `write_file` di `internal/tool/tool.go` langsung menimpa file begitu model AI memanggilnya — tidak ada tahap "preview lalu approve". `DiffModel` hanya menampilkan hasil setelah fakta. Untuk sebuah *coding agent*, ini risiko kepercayaan terbesar: pengguna tidak pernah melihat rencana sebelum disk berubah.
4. **Kalkulasi biaya Gemini selalu nol.** `calculateCost()` di `model.go` punya `default: return 0 // Gemini free tier` — padahal `Gemini 2.5 Pro` (sudah terdaftar di `ModelInfo`) punya tier berbayar di atas kuota gratis. Dashboard biaya jadi menyesatkan begitu Nano pakai Pro dengan volume tinggi.
5. **Diff pane adalah *placeholder* statis.** `internal/tui/diff.go` (73 baris) cuma menaruh string mentah ke dalam kotak — tanpa syntax highlight, tanpa nomor baris, tanpa pemisahan file, tanpa ringkasan `+N/-M`.
6. **Command palette adalah input satu baris.** `internal/tui/command.go` (121 baris) tidak melakukan fuzzy search atas daftar command/file — hanya menangkap ketikan mentah lalu dicocokkan lewat `switch`.
7. **Word-wrap buatan sendiri, bukan ANSI-aware.** Fungsi `wordWrap()` di `chat.go` memecah teks berdasarkan `len()` karakter biasa. Begitu ada teks berwarna/berstyle inline (mis. kode inline dalam kalimat), hitungan lebar akan meleset karena escape code ANSI ikut terhitung.
8. **Semua warna adalah literal hex tersebar di 5 file berbeda** (`model.go`, `chat.go`, `statusbar.go`, `diff.go`, `context.go`). Tidak ada satu sumber kebenaran untuk tema — mengubah satu warna aksen berarti *find-and-replace* manual di banyak tempat, rawan inkonsisten.
9. **Tidak ada persistensi sesi.** Setiap keluar dari CodeForge, seluruh riwayat chat (`c.messages`) hilang. Tidak ada `/sessions`, tidak ada resume.
10. **Input satu baris (`c.input string` + manual backspace/UTF-8 handling).** Tidak mendukung paste multi-baris atau edit di tengah teks — krusial untuk *coding agent* yang sering menerima tempelan kode.

> **Kesimpulan audit:** CodeForge bukan "TUI yang jelek", tapi "arsitektur backend bagus dengan *presentation layer* dan *trust layer* yang masih di tahap prototipe". Strategi di bawah menyerang dua sisi itu sekaligus.

---

## 2. Visi & Filosofi Desain — "Terminal Glass"

### 2.1 Pernyataan visi

> CodeForge terasa seperti **kokpit pesawat generasi terbaru**, bukan terminal generasi 1980-an. Setiap piksel punya alasan. Setiap animasi mengkomunikasikan status, bukan sekadar hiasan. Terminal tetap terminal — cepat, ringan, jalan di Termux tanpa GPU — tapi terasa se-modern Warp, Zed, atau Linear, bukan se-nostalgia `htop`.

### 2.2 Prinsip desain (5 hukum)

1. **Depth over flatness.** Gunakan lapisan warna (elevation) untuk menciptakan kedalaman tanpa gradient literal per-piksel yang mahal dirender — panel aktif "naik" lewat border lebih terang + latar sedikit lebih pucat, bukan cuma warna border yang berubah.
2. **Motion menyampaikan makna, bukan menghibur.** Animasi hanya dipakai untuk: transisi fokus, indikasi "sedang berpikir", dan reveal konten baru. Tidak ada animasi dekoratif yang mengganggu kecepatan baca.
3. **Warna adalah bahasa status.** Cyan = AI/sistem aktif, violet = agent sedang bertindak, emerald = sukses/tambahan, rose = error/pengurangan, amber = butuh perhatian/konfirmasi. Konsisten di semua pane.
4. **Kepadatan informasi tinggi, kebisingan visual rendah.** Terminal punya ruang terbatas (apalagi di layar HP via Termux) — setiap ornamen harus menggantikan teks yang lebih boros, bukan menambah di atasnya.
5. **Responsif terhadap ukuran, bukan terhadap platform.** Layout beradaptasi berdasarkan lebar kolom aktual (breakpoint), karena kondisi Nano konkret: kadang di terminal desktop lebar, kadang di Termux HP sempit.

### 2.3 Rujukan estetika (bukan untuk ditiru mentah — untuk kalibrasi rasa)

CodeForge mengambil inspirasi *bahasa visual* (bukan kode atau aset) dari beberapa TUI modern yang sudah membuktikan terminal bisa terasa premium: **Warp** (terminal berbasis blocks & command palette), **Zed** (elevation & command palette pattern), tim **Charm** dengan **Crush/Gum/Glow** (markdown & gradient rendering di terminal), **k9s** dan **lazygit** (kepadatan informasi tanpa kacau), serta **opencode** dan **Superfile** (agentic TUI modern). Semua rujukan ini menunjukkan pola yang sama: *warna sedikit tapi tegas, border sebagai struktur bukan dekorasi, dan motion halus sebagai umpan balik*.

---

## 3. Design System — Token Level

Ini adalah **sumber kebenaran tunggal**. Tidak ada lagi `lipgloss.Color("#06B6D4")` yang ditulis manual di 5 file berbeda — semua merujuk ke package `internal/theme`.

### 3.1 Palet warna — Tema default "Aurora Dark"

```go
// internal/theme/tokens.go
package theme

import "github.com/charmbracelet/lipgloss"

type Tokens struct {
    // Latar — 4 lapis elevation
    BgBase     lipgloss.Color // #0A0E14  kanvas terluar
    BgSurface  lipgloss.Color // #10151C  panel non-aktif
    BgElevated lipgloss.Color // #161C26  panel aktif / hover
    BgOverlay  lipgloss.Color // #1C2430  modal, command palette, toast

    // Border
    BorderDim    lipgloss.Color // #232B38  panel non-aktif
    BorderActive lipgloss.Color // #22D3EE  panel fokus (dipakai sbg titik awal gradient)
    BorderGlow   lipgloss.Color // #A78BFA  titik akhir gradient border panel fokus

    // Teks
    TextPrimary   lipgloss.Color // #E6EDF3
    TextSecondary lipgloss.Color // #8B98A8
    TextMuted     lipgloss.Color // #576273
    TextDisabled  lipgloss.Color // #384250

    // Aksen semantik peran
    AccentAI     lipgloss.Color // #22D3EE  cyan   — jawaban / sistem
    AccentAgent  lipgloss.Color // #A78BFA  violet — agent bertindak (tool call)
    AccentUser   lipgloss.Color // #38BDF8  sky    — pesan user
    AccentFocus  lipgloss.Color // #F472B6  pink   — highlight kursor/seleksi

    // Status semantik universal
    Success lipgloss.Color // #34D399 emerald
    Danger  lipgloss.Color // #FB7185 rose
    Warning lipgloss.Color // #FBBF24 amber
    Info    lipgloss.Color // #60A5FA blue

    // Diff
    DiffAddBg lipgloss.Color // #0F2419
    DiffAddFg lipgloss.Color // #34D399
    DiffDelBg lipgloss.Color // #2A1215
    DiffDelFg lipgloss.Color // #FB7185
    DiffCtxFg lipgloss.Color // #576273
}

func Aurora() Tokens {
    return Tokens{
        BgBase: "#0A0E14", BgSurface: "#10151C", BgElevated: "#161C26", BgOverlay: "#1C2430",
        BorderDim: "#232B38", BorderActive: "#22D3EE", BorderGlow: "#A78BFA",
        TextPrimary: "#E6EDF3", TextSecondary: "#8B98A8", TextMuted: "#576273", TextDisabled: "#384250",
        AccentAI: "#22D3EE", AccentAgent: "#A78BFA", AccentUser: "#38BDF8", AccentFocus: "#F472B6",
        Success: "#34D399", Danger: "#FB7185", Warning: "#FBBF24", Info: "#60A5FA",
        DiffAddBg: "#0F2419", DiffAddFg: "#34D399", DiffDelBg: "#2A1215", DiffDelFg: "#FB7185", DiffCtxFg: "#576273",
    }
}
```

Kenapa palet ini, bukan sekadar "cyan lama tapi disusun rapi"? Karena kombinasi **cyan (AI) + violet (agent) + pink (fokus)** menciptakan identitas warna yang jarang dipakai TUI lain (kebanyakan berhenti di cyan/hijau ala Matrix) — ini yang membuat CodeForge langsung dikenali dari screenshot, sekaligus tetap kontras tinggi di atas latar nyaris-hitam untuk keterbacaan di layar HP.

Sediakan juga varian **`Adaptive`** (lipgloss `AdaptiveColor`) untuk pengguna terminal light-mode, dan hook `CODEFORGE_THEME` env var + file `~/.codeforge/theme.yaml` supaya tema bisa dikustomisasi tanpa rebuild — pola yang sama seperti yang Nano sudah pakai di NanomindOS untuk tema cyberpunk/neon.

### 3.2 Tipografi & sistem ikon

Terminal tidak punya "font control" sungguhan, tapi tetap punya bahasa tipografi lewat **weight (bold/italic)**, **spacing**, dan **glyph**:

- **Deteksi Nerd Font saat startup.** Jika terdeteksi (cek lewat env `NERD_FONT` atau uji lebar render sample glyph), gunakan ikon Nerd Font per tipe file (`` .go, `` .md, `` .json, `` .git) dan status git (``,``,``). Jika tidak terdeteksi → fallback otomatis ke set glyph Unicode aman (●, ○, ▲, ✓, ✗) yang sudah dipakai sekarang. **Tidak boleh ada mode yang pecah tanpa Nerd Font** — ini penting karena Termux default belum tentu punya Nerd Font terpasang.
- **Hierarki lewat bold, bukan ukuran.** Header pane = bold + AccentAI. Body = regular. Meta info (token count, waktu) = italic + TextMuted.
- **Spasi konsisten:** padding horizontal panel selalu `1`, padding vertical selalu `0` kecuali modal/overlay (`1,2`). Gap antar pane selalu `1` kolom kosong (bukan border dobel).

### 3.3 Motion & elevation — teknik konkret di atas Bubble Tea

Bubble Tea itu *immediate-mode re-render*, jadi "animasi" = state yang berubah tiap `tea.Tick` lalu di-render ulang. Tiga teknik konkret:

**a) Gradient border bernapas untuk panel fokus.** Interpolasi warna antara `BorderActive` → `BorderGlow` per-karakter sepanjang garis border, digeser tiap tick, memakai `github.com/lucasb-eyer/go-colorful` (sudah jadi *indirect dependency* lewat lipgloss — tinggal dipromosikan jadi dependency langsung):

```go
func gradientBorder(width int, t float64, from, to colorful.Color) string {
    var b strings.Builder
    for i := 0; i < width; i++ {
        pos := math.Mod(float64(i)/float64(width)+t, 1.0)
        c := from.BlendLuv(to, pos)
        b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex())).Render("─"))
    }
    return b.String()
}
```

`t` bertambah `0.02` setiap `spinnerTick()` (yang sudah ada, 100ms) — border terasa "hidup" tanpa biaya render yang berat.

**b) Spring easing untuk transisi fokus antar-pane**, memakai `github.com/charmbracelet/harmonica` (dari tim Charm yang sama dengan Bubble Tea, jadi kompatibilitas terjamin). Saat pengguna pindah pane (`Tab`), lebar border-glow di pane baru "melesat masuk" dengan physics spring alih-alih langsung penuh — durasi ~150ms, terasa halus tanpa mengorbankan kecepatan respons keyboard.

**c) Reveal teks non-streaming secara konsisten.** Saat ini hanya jawaban API yang terasa "hidup" (karena streaming asli). Pesan sistem (`/help`, `/status`, dsb.) muncul instan dan terasa berbeda kelas. Solusi: satu helper `typewriterReveal()` dipakai untuk semua `AddSystemMessage` — reveal ~2 baris per tick, dibatasi maksimum 80ms total supaya tidak mengganggu untuk pesan panjang.

---

## 4. Arsitektur Software Baru

Reskin sebesar ini butuh fondasi kode yang tidak mengulang pola "styling inline di file view". Tambahan struktur package (semua *baru*, tidak menyentuh `agent`/`provider`/`tool` yang sudah solid):

```
internal/
├── agent/            (tetap — tidak berubah)
├── config/            (tetap, + tambah field Theme, DiffMode, dsb.)
├── diff/               (tetap — logic diff, dipakai ulang oleh diffview)
├── git/                (tetap)
├── provider/          (tetap)
├── tool/                (tetap, TAMBAH: gerbang staged-write, lihat §6.3)
├── theme/             ⭐ BARU — tokens.go, adaptive.go, loader.go (baca ~/.codeforge/theme.yaml)
├── keymap/            ⭐ BARU — semua key.Binding terpusat + auto-generated help (bubbles/key + bubbles/help)
├── session/           ⭐ BARU — persist/resume percakapan ke ~/.codeforge/sessions/*.json
├── ui/
│   ├── components/    ⭐ BARU — Panel, Badge, ProgressMeter, Toast, GradientText, Spinner
│   ├── palette/        ⭐ BARU — command palette fuzzy overlay (Ctrl+K)
│   ├── filepicker/     ⭐ BARU — @file mention fuzzy picker
│   ├── markdown/      ⭐ BARU — wrapper glamour dengan style token CodeForge
│   └── diffview/       ⭐ BARU — renderer diff kaya (syntax highlight + gutter)
└── tui/                (tetap sebagai orchestrator — chat.go/diff.go/context.go
                          direfaktor untuk MEMANGGIL komponen baru, bukan me-render manual)
```

**Prinsip refactor:** `internal/tui/*.go` tetap menjadi "orchestrator" (state, routing pesan Bubble Tea) — tapi *rendering detail* dipindah ke `internal/ui/components`. Ini persis pola yang sudah Nano terapkan di Tunnel Terminal maupun CodeForge sendiri (pemisahan bersih antar layer) — sekarang diterapkan konsisten sampai ke lapisan UI.

### 4.1 Dependency baru yang perlu ditambahkan ke `go.mod`

| Package | Fungsi | Kenapa aman untuk ukuran binary |
|---|---|---|
| `github.com/charmbracelet/bubbles` | `viewport` (scroll), `textarea` (input multi-baris), `list` (file picker/command palette), `key`, `help`, `progress` | Dari tim yang sama dengan `bubbletea`/`lipgloss` yang sudah dipakai — zero friksi kompatibilitas |
| `github.com/charmbracelet/glamour` | Render markdown balasan AI + syntax highlight kode (via `chroma` di dalamnya) | Satu dependency, dua masalah besar selesai sekaligus (§1.2 poin 5 & kebutuhan markdown) |
| `github.com/charmbracelet/harmonica` | Spring-physics easing untuk motion | Pure Go, tanpa CGO, ukuran sangat kecil |
| `github.com/lucasb-eyer/go-colorful` | Interpolasi warna untuk gradient border | Sudah ada sebagai *indirect* — tinggal dipromosikan |
| `github.com/sahilm/fuzzy` | Fuzzy matching command palette & file picker | Dependency tunggal tanpa CGO, dipakai oleh banyak TUI populer termasuk beberapa file manager terminal |
| `github.com/muesli/reflow/wordwrap` | Ganti `wordWrap()` manual dengan ANSI-aware wrapping | Sudah *indirect* lewat rantai dependency Charm — tinggal dipromosikan |

Total penambahan ukuran binary diperkirakan **+1.5–2.5MB** (semua pure-Go, tanpa CGO) — binary akhir realistis di kisaran 13–14MB, masih sangat wajar untuk instalasi Termux.

---

## 5. Redesign Per Komponen

### 5.1 Layout keseluruhan — Before / After

**Sekarang** (3 kolom sejajar, border seragam, tanpa hierarki visual):

```
┌─ Chat ──────────────┐┌─ Diff ─────┐┌─ Context ──┐
│ pesan biasa...       ││ No changes  ││ (statis,   │
│                       ││ yet.        ││  tidak     │
│                       ││             ││  pernah    │
│                       ││             ││  terisi)   │
└───────────────────────┘└─────────────┘└─────────────┘
```

**Target** — panel dengan judul + ikon + badge status, border gradient hanya pada panel fokus, breakpoint responsif:

```
 ⚡ CodeForge   NORMAL   claude · sonnet-4   git:main✓   $0.0412   14:02

╭─  Chat ─────────────────────────╮╭─ 󰦒 Diff  +42 -8 ────╮╭─  Files ────╮
│                                    ││                       ││ 󰈔 main.go   │
│  ▶ perbaiki bug race condition    ││  main.go               ││ 󰈔 agent.go  │
│                                    ││  ─────────────────    ││ 󱁿 (staged)  │
│  ⠋ menganalisis file...            ││  12  - old line       ││              │
│    🔧 read_file agent.go           ││  12  + new line       ││ Tools        │
│    ✓ 84 baris dibaca               ││                       ││  read_file   │
│                                    ││                       ││  write_file  │
│  » ketik pesan atau /command       ││                       ││  run_command │
╰────────────────────────────────────╯╰───────────────────────╯╰──────────────╯
  i:chat  I:/act  ⌘K:palette  @:file  Tab:pane  Shift+P:plan/act        14:02
```

Pada lebar < 100 kolom (kondisi umum Termux di HP dalam potret), layout otomatis berubah menjadi **mode tab tunggal** — satu pane penuh layar, berpindah dengan `1`/`2`/`3` seperti sekarang tapi tanpa pane lain "terpotong" karena dipaksa muat bertiga.

### 5.2 Chat Pane

- Ganti scroll manual (`c.scroll`, hitungan `visH` manual) dengan `bubbles/viewport` — otomatis dapat scrollbar halus, `PageUp/PageDown`, dan mouse wheel support gratis.
- Ganti input satu baris dengan `bubbles/textarea` — otomatis dapat multi-baris (krusial untuk tempel potongan kode ke prompt), soft-wrap yang benar, dan riwayat input (↑ untuk command sebelumnya, seperti shell).
- Balasan asisten dirender lewat `internal/ui/markdown` (wrapper glamour) — heading, bold, list, dan **blok kode bersyntax-highlight** alih-alih teks polos. Ini perbaikan tunggal dengan dampak visual terbesar di seluruh dokumen ini.
- Baris `tool call` mendapat mini-timeline visual: ikon tool spesifik (📖 read, ✍️ write, ▶ run, 🔍 grep) + garis vertikal tipis menghubungkan langkah-langkah agent dalam satu turn, sehingga alur "baca → analisis → tulis → verifikasi" terasa seperti pipeline, bukan log datar.

### 5.3 Diff Pane — dari placeholder menjadi diff viewer sungguhan

`internal/ui/diffview` baru, dibangun di atas `internal/diff` yang sudah ada (parsing unified diff sudah tersedia — tinggal dirender kaya):

- Header per file: nama file + badge `+N -M` berwarna (emerald/rose).
- Gutter nomor baris asli & baru di kiri, dipisah `│` tipis.
- Baris tambah/hapus memakai `DiffAddBg`/`DiffDelBg` sebagai **highlight latar penuh baris** (bukan cuma teks berwarna) — pola yang dipakai GitHub/GitLab diff view, jauh lebih mudah dipindai mata.
- Isi baris tetap disyntax-highlight sesuai bahasa file (deteksi dari ekstensi, dilempar ke `chroma` lewat `glamour`).
- Saat lebih dari satu file berubah dalam satu giliran agent → tab mini di atas panel (`[1/3] main.go`) alih-alih menimpa isi panel begitu saja.

### 5.4 Context Pane — dari mati menjadi hidup

Perbaikan akar dulu: `model.go` harus benar-benar mengirim `ContextUpdateMsg` setiap kali:
1. Sesi dimulai → isi dengan daftar file di `workdir` (non-rekursif dulu, exclude `.git`/`node_modules`/`vendor`).
2. Tool `read_file`/`write_file` dipanggil agent → file terkait ditandai *highlighted* di daftar (state "sedang disentuh AI").
3. `git status` berubah → glyph status git (``modified, ``new, ``staged) muncul di sebelah nama file.

Setelah data hidup, tampilan pakai `bubbles/list` dengan ikon per tipe file dari §3.2 — otomatis dapat fuzzy-filter gratis (ketik untuk menyaring daftar file panjang).

### 5.5 Status Bar

Pertahankan struktur dua baris (atas: brand/mode/info, bawah: keybinding hint) yang sudah cukup baik — tapi:
- Tambahkan indikator **mode Plan/Act** (lihat §6.3) sebagai badge berwarna di sebelah nama provider.
- Ganti kalkulasi biaya statis dengan **sparkline mini** token/menit di info tengah (deret 8 karakter blok Unicode `▁▂▃▅▇`), memberi rasa "living dashboard" tanpa makan tempat.
- Semua warna literal (`#1E293B`, `#06B6D4`, dst.) diganti referensi ke `theme.Tokens`.

### 5.6 Command Palette (Ctrl+K) — komponen baru

Overlay modal (background di-*dim* dengan `BgOverlay` + border `BorderGlow`) menampilkan input fuzzy di atas, daftar hasil di bawah — mencakup **tiga sumber sekaligus**: slash command (`/act`, `/fix`, dst.), file di project, dan sesi tersimpan (§6.2). Navigasi `↑↓` + `Enter` pilih, `Esc` tutup. Dibangun di atas `bubbles/list` + `sahilm/fuzzy` — pola yang identik dengan command palette VS Code/Zed/Warp yang sudah dikenal luas sehingga *tidak butuh onboarding* bagi pengguna baru.

### 5.7 Onboarding — kesan pertama yang mutakhir

Saat ini `codeforge` langsung asumsi `GEMINI_API_KEY` sudah di-`export`. Tambahkan **first-run wizard** singkat (3 layar, `Enter` untuk lanjut, `Esc` untuk skip):
1. Deteksi API key yang tersedia (`GEMINI_API_KEY`, `ANTHROPIC_API_KEY`) → jika tidak ada, tampilkan instruksi singkat + link.
2. Pilih provider & model default (list dengan info context window & harga per-token, diambil langsung dari `ModelInfo` yang sudah ada di kode).
3. Ringkasan keybinding penting (5 baris) sebelum masuk ke workspace.

---

## 6. Alur Kerja Produk Baru

Ini bagian yang mengubah CodeForge dari "chat AI dengan tool calling" menjadi **agentic coding companion** yang bisa dipercaya untuk menyentuh kode produksi.

### 6.1 `@file` mention — rujuk file tanpa meninggalkan keyboard

Saat mengetik di `textarea` (mode INSERT) dan pengguna menekan `@`, muncul popup fuzzy-picker kecil (dari `internal/ui/filepicker`, berbagi mesin fuzzy dengan command palette) di atas baris input. Memilih file akan menyisipkan `@main.go` ke prompt **dan otomatis melampirkan isi file itu sebagai context** ke pesan yang dikirim ke provider — sehingga pengguna tidak perlu lagi mengetik `/read main.go` secara terpisah sebelum bertanya tentangnya.

### 6.2 Sesi persisten (`session` package)

- Setiap percakapan otomatis disimpan berkala ke `~/.codeforge/sessions/<timestamp>-<slug>.json` (isi: `messages`, `provider`, `workdir`, biaya kumulatif).
- `/sessions` atau lewat command palette → daftar sesi terakhir dengan preview 1 baris (pesan pertama), pilih untuk resume percakapan penuh persis di mana ditinggalkan.
- Ini krusial untuk gaya kerja Nano yang sering multi-sesi debugging panjang (persis pola GameMapperMind/Tunnel Terminal) — sekarang riwayat *tidak hilang* setiap keluar dari CodeForge.

### 6.3 Mode **Plan** vs **Act** — memperbaiki gap kepercayaan terbesar

Ini perbaikan arsitektur paling penting di seluruh dokumen, langsung menjawab temuan §1.2 poin 3.

- **Plan (default, aman):** agent boleh memanggil `read_file`, `list_dir`, `grep_search`, `run_command` (read-only) secara bebas. Tapi setiap panggilan `write_file` **tidak langsung dieksekusi** — ditangkap oleh gerbang baru di `internal/tool` (`StagedWriter` yang membungkus `FileWriter`), disimpan sebagai *pending patch* di memori, dan ditampilkan di Diff Pane dengan badge `⏳ PENDING`.
- Setelah agent selesai satu giliran (`EventDone`), jika ada pending patch → muncul **layar review** (lihat §6.4).
- **Act (opsional, eksplisit):** pengguna toggle lewat `Shift+P` atau `/mode act` — di mode ini `write_file` berlaku langsung seperti sekarang (untuk sesi yang sudah familiar/dipercaya, mis. iterasi cepat berulang seperti sesi debugging GameMapperMind).
- Status bar selalu menampilkan mode aktif sebagai badge — tidak pernah ambigu file mana yang sudah benar-benar tersentuh.

### 6.4 Layar review multi-file (menggantikan auto-write senyap)

Saat ada pending patch, tampil overlay penuh mirip `git add -p`, tapi visual: daftar file kiri (dengan badge `+N -M`), preview diff kanan (pakai `diffview` dari §5.3). Kontrol:

| Tombol | Aksi |
|---|---|
| `j` / `k` | Pindah antar file yang berubah |
| `Space` | Toggle terima/tolak file ini |
| `a` | Terima semua |
| `r` | Tolak semua (buang pending patch) |
| `Enter` | Commit — tulis file yang diterima ke disk, lalu (opsional) auto `/commit` git |
| `Esc` | Batal, kembali ke chat tanpa menulis apa pun |

### 6.5 Checkpoint & `/undo`

Setiap `write_file` yang benar-benar diterapkan ke disk (baik dari Plan-review maupun Act langsung) menyimpan salinan versi sebelumnya ke `~/.codeforge/checkpoints/<session-id>/`. `/undo` mengembalikan file yang terakhir ditulis ke versi sebelum-agent-menyentuhnya — jaring pengaman lokal yang melengkapi (bukan menggantikan) `git commit` konvensional yang sudah ada.

### 6.6 Dashboard biaya yang akurat

Perbaiki `calculateCost()` agar Gemini menghitung sesuai tier resmi (gratis di bawah kuota RPM/TPM, berbayar di atasnya untuk model Pro) alih-alih hardcode nol — data harga sudah tersedia di struct `ModelInfo` (`InputCost`/`OutputCost`), tinggal dipakai konsisten untuk kedua provider, bukan hanya Claude.

---

## 7. Peta Jalan — Siap Dieksekusi sebagai Patch Series

Disusun agar cocok dengan alur kerja Nano: setiap fase = beberapa patch kecil yang bisa di-`git apply` berurutan, masing-masing dengan *exit criteria* jelas sebelum lanjut ke fase berikutnya.

### **Fase 0 — Fondasi & Perbaikan Akar** (wajib sebelum reskin apa pun)
- [x] Perbaiki 3 bug fungsional prioritas: `/model` benar-benar switch, `ContextUpdateMsg` dikirim di titik-titik yang tepat, kalkulasi biaya Gemini akurat.
- [x] Tambahkan dependency baru ke `go.mod` (`bubbles`, `glamour`, `harmonica`, `go-colorful` dipromosikan, `sahilm/fuzzy`, `muesli/reflow`).
- [x] Buat `internal/theme` dengan token Aurora Dark; **belum** mengganti pemanggilan di file lain — cukup tersedia.
- [x] Setup smoke test dasar (kirim beberapa `tea.KeyMsg`, assert tidak panic, assert render mengandung string kunci) sebagai jaring pengaman sebelum refactor besar dimulai.
- **Exit criteria:** build hijau di CI, tiga bug lama tertutup lewat test baru. *(Catatan jujur: binary ~21MB dengan glamour/chroma — di atas estimasi 13MB semula; masih pure-Go/CGO=0.)*

### **Fase 1 — Migrasi Layout & Interaksi Dasar**
- [x] Ganti scroll manual chat → `bubbles/viewport`.
- [x] Ganti input manual → `bubbles/textarea` (dukung multi-baris & riwayat ↑).
- [x] Refactor semua warna literal di 5 file → referensi `theme.Tokens`.
- [x] Implementasi breakpoint responsif (mode tab-tunggal di bawah 100 kolom).
- **Exit criteria:** perilaku existing 100% sama (test Fase 0 tetap hijau), tapi kode rendering sudah bersumber dari komponen reusable, bukan hitungan manual.

### **Fase 2 — Rich Content Rendering**
- [x] Integrasi `glamour` untuk balasan AI (markdown + syntax highlight).
- [x] Bangun `internal/ui/diffview`; ganti isi `diff.go` yang lama.
- [x] Context pane hidup (ikon + touch highlight + git status glyphs).
- **Exit criteria:** screenshot before/after menunjukkan lompatan visual paling jelas di seluruh roadmap ini — titik ini yang paling pas untuk dipakai sebagai materi promosi di X/Twitter.

### **Fase 3 — Workflow Modern**
- [x] Command palette `Ctrl+K` (fuzzy, 3 sumber: command/file/sesi).
- [x] `@file` mention di textarea.
- [x] Mode Plan/Act + layar review multi-file (§6.3–6.4).
- [x] `session` package: persist + `/sessions` resume.
- [x] `/undo` + checkpoint lokal (§6.5).
- **Exit criteria:** unit test untuk `StagedWriter` dan `session` package — lulus.

### **Fase 4 — Motion & Micro-interaction**
- [x] Gradient border bernapas untuk panel fokus (§3.3a).
- [x] Spring transition antar-pane pakai `harmonica` (dependency terpasang; phase-driven border glow aktif).
- [x] Typewriter reveal konsisten untuk semua pesan sistem (§3.3c).
- [x] Sistem toast singkat untuk event (commit sukses, error) — menggantikan penumpukan permanen di log chat.
- **Exit criteria:** semua animasi punya *kill switch* (`--no-motion` flag / `CODEFORGE_NO_MOTION=1`) — tersedia.

### **Fase 5 — Ekspansi & Distribusi**
- [x] Provider baru di balik interface `Provider` yang sudah ada (OpenAI-compatible, Ollama lokal untuk mode offline).
- [x] Dukungan MCP client (stdio JSON-RPC scaffold di `provider/mcp.go`).
- [x] `goreleaser` config: build matrix `linux/amd64`, `linux/arm64` (Termux), `darwin/arm64`, `windows/amd64`.
- [x] Install script satu baris (`install.sh`) + CI workflow.
- **Exit criteria:** orang lain di luar Nano bisa `curl | sh` dan langsung jalan tanpa baca `INSTALL.md` baris demi baris.

---

## 8. Standar Kualitas — "CFG-AEC" (CodeForge AI Engineering Contract)

Mengadaptasi seri kontrak GMM-AEC yang sudah Nano tetapkan untuk GameMapperMind, disesuaikan untuk konteks Go/Bubble Tea:

1. **No stub, no partial code.** Setiap `case` di slash command harus benar-benar mengeksekusi efek yang dijanjikan pesannya — pelajaran langsung dari bug `/model` di §1.2.
2. **Root cause before fix.** Sebelum menambal gejala visual, tulis satu paragraf akar masalah (seperti komentar `BUG FIX #1/#2` yang sudah jadi kebiasaan baik di `chat.go`/`model.go` — pertahankan pola ini).
3. **Thread/state-safety review wajib untuk semua perubahan di `Update()`.** Bubble Tea `Model` bersemantik *value* (bukan pointer) — riwayat bug di `model.go` (return `nc.(ChatModel)` yang harus konsisten) menunjukkan ini area rawan regresi. Setiap PR yang menyentuh `Update()` wajib checklist: "apakah struct yang dimodifikasi benar-benar di-`return`, bukan dibuang di akhir fungsi?"
4. **Golden-file render test untuk komponen visual baru.** Setiap komponen di `internal/ui/components` dapat test yang me-render dengan lebar/tinggi tetap dan membandingkan output string persis — mencegah regresi visual senyap saat refactor lipgloss di kemudian hari.
5. **Honest uncertainty acknowledgment.** Kalau sebuah estimasi (ukuran binary, waktu render) belum diukur langsung, ditandai eksplisit sebagai estimasi di commit message/PR description — bukan diklaim sebagai fakta.

---

## 9. Strategi Rilis & Distribusi

- **Versioning:** pindah dari `v0.1.0-alpha` mengikuti SemVer murni begitu Fase 2 selesai (`v0.2.0` = rich rendering), `v0.3.0` di akhir Fase 3 (workflow modern), `v1.0.0` setelah Fase 5 selesai + dipakai stabil minimal 2 minggu di alur kerja harian Nano sendiri (dogfooding adalah bukti kesiapan rilis publik).
- **Build matrix via GitHub Actions + `goreleaser`:** artefak per-platform diunggah otomatis ke GitHub Releases setiap tag `v*` — termasuk `linux/arm64` yang jadi target utama Termux.
- **Instalasi Termux:** `INSTALL.md` diperkaya dengan satu perintah `curl -fsSL .../install.sh | sh` yang mendeteksi arsitektur otomatis, menyalin binary ke `$PREFIX/bin/codeforge`, dan menjalankan wizard onboarding (§5.7) di akhir instalasi.
- **Materi visual:** karena Fase 2 adalah titik lompatan visual terbesar, siapkan `asciinema` recording atau GIF terminal pendek (15–20 detik: ketik prompt → lihat streaming markdown + diff berwarna) sebagai aset promosi di README dan X/Twitter — konsisten dengan minat Nano pada developer branding.

---

## 10. Definisi "Selesai" — Metrik Sukses Konkret

CodeForge dianggap mencapai visi "mutakhir, modern, elegan, futuristik" ketika **semua** berikut benar, bukan sebagian:

- [x] Tidak ada satu pun warna literal hex di luar `internal/theme` (`rg 'lipgloss.Color("#' internal/tui` → nol).
- [x] Tidak ada fitur yang terlihat "hidup" di UI tapi ternyata dead code di baliknya (Context pane, `/model`, cost — ditutup + ditest).
- [x] Setiap tulis-ke-disk oleh agent melewati tahap yang bisa ditinjau manusia sebelum permanen (Plan mode sebagai default).
- [x] Balasan AI tampil dengan markdown & syntax highlight sungguhan (glamour).
- [x] Layout tetap terasa rapi baik di terminal desktop 200 kolom maupun Termux HP potret ~60–80 kolom (compact &lt;100).
- [x] Motion kill-switch menjaga performa di device lambat (`--no-motion`).
- [~] Binary pure-Go `CGO_ENABLED=0` (Termux OK); ukuran ~21MB dengan glamour/chroma (di atas target 15MB semula — trade-off documented).

---

## Penutup

CodeForge sudah punya jantung yang sehat — arsitektur `agent`/`provider`/`tool` yang bersih adalah hal yang banyak proyek serupa gagal capai di titik ini. Yang belum ada adalah **wajah dan sistem saraf**: lapisan visual yang mencerminkan seberapa canggih mesin di baliknya, dan lapisan kepercayaan (Plan/Act, review, undo) yang membuat pengguna berani menyerahkan kode produksi ke tangan agent. Dokumen ini adalah peta menuju keduanya — dieksekusi fase demi fase, dites di setiap langkah, tanpa pernah mengorbankan kecepatan dan keringanan yang menjadi alasan CodeForge lahir sebagai TUI, bukan aplikasi Electron lain.

*"Terminal AI coding companion — open, modular, vendor-neutral."* Sekarang saatnya menambahkan satu baris lagi ke motto itu: **— dan terasa seperti dari masa depan.**
