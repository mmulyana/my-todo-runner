# my-todo-runner

Runner agentic loop untuk my-todo. Tiap jam (systemd timer) runner:

1. Query todo `assignee = agent` yang belum selesai lewat MCP my-todo.
2. Ambil **satu** kartu (paling lama dibuat) yang kolom kanban-nya ada di `pickFrom` (default `Ready`), lalu geser ke `In Progress`.
3. Siapkan worktree di branch `agent/<id8>-<slug>` dari `origin/<base>`, jalankan `opencode run`.
4. Agent selesai → push, buka PR, attach URL PR ke todo, catat ringkasan di note, geser ke `Review`.
   Agent buntu / timeout / gagal → catat alasan di note, geser ke `Blocked`.

Kartu manual (`assignee` kosong) tidak pernah disentuh. Agent tidak pernah merge.

## Kontrak dengan agent

Agent menulis JSON ke `$AGENT_RESULT_PATH`:

```json
{ "status": "done", "prTitle": "feat: ...", "summary": "..." }
{ "status": "blocked", "question": "..." }
```

Prompt default ada di `internal/agent/prompt.md` (di-embed ke binary); bisa diganti lewat `promptFile` di config. Kontrak ini harness-agnostic, jadi bisa dipakai harness lain selain OpenCode.

## Syarat di my-todo

- Backend sudah punya kolom `assignee` (migration `0019_add-todo-assignee`).
- Project punya kolom board `Ready`, `In Progress`, `Review`, `Blocked` (nama bisa diubah di config).
- Kalau project punya lebih dari satu repo, note kartu wajib berisi baris `repo: <alias>`, misalnya `repo: be`.
- Tandai kartu untuk agent: `update_todo` dengan `assignee: "agent"`.

## Setup VPS

```bash
# 1. user khusus + direktori kerja
sudo useradd -m -s /bin/bash agent
sudo mkdir -p /srv/agent /etc/my-todo-runner
sudo chown agent:agent /srv/agent

# 2. tools: git, gh, opencode (sebagai user agent)
sudo apt install -y git gh
sudo -iu agent bash -c 'curl -fsSL https://opencode.ai/install | bash'
# catat path binary opencode (biasanya /home/agent/.opencode/bin/opencode) untuk `opencodeBin`

# 3. auth GitHub: token fine-grained, hanya repo di config, izin Contents + Pull requests (read/write)
sudo -iu agent bash -c 'echo "$GH_TOKEN" | gh auth login --with-token && gh auth setup-git'

# 4. binary runner (build di lokal: `make build`)
scp dist/my-todo-runner vps:/tmp/ && ssh vps 'sudo install -m 755 /tmp/my-todo-runner /usr/local/bin/'

# 5. config + secret
sudo cp deploy/config.example.json /etc/my-todo-runner/config.json   # sesuaikan opencodeBin & projects
sudo cp deploy/env.example /etc/my-todo-runner/env                   # isi token
sudo chown root:agent /etc/my-todo-runner/env && sudo chmod 640 /etc/my-todo-runner/env

# 6. systemd
sudo cp deploy/my-todo-runner.service deploy/my-todo-runner.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now my-todo-runner.timer
```

### OpenCode (user `agent`)

Runner memanggil `opencode run --auto --standalone "<prompt>"` (bisa diubah lewat `opencodeArgs`):

- `--auto` menyetujui izin edit/bash otomatis, jadi tidak perlu konfigurasi `permission`.
- `--standalone` memakai server privat per run, jadi tidak bergantung pada background service.
- MCP my-todo **disuntik otomatis** lewat env `OPENCODE_CONFIG_CONTENT` dari `MY_TODO_MCP_URL` + `MY_TODO_TOKEN`; token tidak perlu ditulis lagi di config OpenCode.
- `PWD` diset ke worktree, karena OpenCode menentukan folder project dari `PWD`, bukan dari working directory proses.

Yang tetap harus disiapkan manual hanya provider/model: login sekali (`sudo -iu agent opencode auth login` atau set API key provider di env), lalu uji `sudo -iu agent opencode run "halo"` harus jalan tanpa menunggu input.

Teruji dengan OpenCode v2.0.18.

### Branch protection

Set di GitHub untuk tiap repo: branch base (`master`/`main`) tidak boleh di-push langsung. Runner hanya push ke `agent/*`.

## Operasional

```bash
# lihat kartu yang akan diambil tanpa mengubah apa pun
sudo -u agent env $(sudo cat /etc/my-todo-runner/env | xargs) my-todo-runner run-once --config /etc/my-todo-runner/config.json --dry-run

# jalankan satu tick sekarang
sudo systemctl start my-todo-runner.service

# log
journalctl -u my-todo-runner.service -f
systemctl list-timers my-todo-runner.timer
```

- Satu tick = maksimal satu kartu; systemd tidak menjalankan tick baru selama tick sebelumnya masih jalan.
- Worktree kartu yang Blocked disimpan di `/srv/agent/work/<todoId>` untuk debugging, dihapus otomatis setelah 7 hari.
- Revisi PR: tulis catatan di note lalu geser kartu kembali ke `Ready`. Kalau PR lama masih open, runner push ulang ke branch yang sama; kalau sudah closed, dibuat PR baru.

## Develop

```bash
make test    # go vet + unit test
make build   # binary linux/amd64 di dist/
```
