Wakeup Bot - Go Version
===

Description
---

**wakeup_go** (module name: `wakebot_go`) is a Telegram bot that sends **Wake-on-LAN (WOL)** magic packets to remote machines. It runs as a long-lived service, listens for Telegram commands, and wakes target PCs on the local network. 

Best fit when you are having a running 24/7 tiny machine like a Raspberry Pi or any embedded platform which supports Linux and Go.

**Requires Go 1.20 and up**.

### Features

- **Multi-Machine Support**: Configure multiple target machines and select them via interactive Telegram reply keyboards.
- **Wake All**: A dedicated "⚡ Wake All" button to broadcast WOL to all configured machines at once.
- **Multi-User Authorization**: Allow multiple users (by Telegram Chat ID) to interact with the bot.
- **Dual-Path WOL**: 
  - **UDP Mode**: Sends standard WOL packets over UDP (No root required).
  - **Raw Mode**: If the machine is fully powered off and ARP fails, it falls back to raw Ethernet frames. Automatically detects physical interfaces and bridges. (Requires Super privileges / `CAP_NET_RAW`).
- **IP Discovery & Caching**: For targets configured with only a MAC address (DHCP), the bot sends raw WOL broadcasts, polls the ARP table to discover the assigned IP when the machine comes online, and caches the IP in memory with a configurable TTL for subsequent faster UDP wakes and ping checks.
- **Async Monitoring**: Notifies you automatically on Telegram when the machine actually comes online after a wake command.

Commands
---

| Command | Description|
| ------- | ---------- |
| `/wake` | Send Magic packet (WOL) to target Machine. If multiple targets are configured, a keyboard appears to select the machine or "⚡ Wake All". |
| `/check` | Check whether the machine is up or not, by sending a `ping` to its IP or performing an ARP lookup. |
| `/list`  | Show the list of available configured machines. |
| `/hello` | Just say "Hi", to make sure that the bot is running properly. |


Configuration
---

See `config.json.template` -> Rename it to `config.json` and edit content, place alongside with the main binary.

| Field            | Description                                                                                                                                                                                                              |
|------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `bot_token`      | Telegram bot token                                                                                                                                                                                                       |
| `chat_ids`       | List of allowed Telegram Chat IDs. (e.g. `[12345678, 87654321]`). Replaces legacy `chat_id`.                                                                                                                             |
| `targets`        | List of target host machines. Each target contains: `name`, `mac` (required), and `ip` (optional, formatted as `ip` or `ip:port`). If IP is omitted, the bot will use raw broadcasts and ARP discovery.                  |
| `wol_passwd`     | Password to send to remote target, if authentication is set (optional)                                                                                                                                                   |
| `ip_cache_ttl`   | How long to cache discovered IPs for machines configured without an IP. Go duration format (e.g., `"12h"`, `"30m"`). Defaults to `"12h"`.                                                                                |
| `api_endpoint`   | Custom Telegram API endpoint (useful for proxies). Defaults to `https://api.telegram.org/bot%s/%s`.                                                                                                                      |
| `remote_ip`      | **(Legacy)** Single target IP. Format: `ip:port`.                                                                                                                                                                        |
| `remote_mac`     | **(Legacy)** Single target MAC. Format: `XX:XX:XX:XX:XX:XX`.                                                                                                                                                             |

*Note on Raw WOL:* To use the Raw Ethernet WOL fallback on Linux, the bot binary needs `CAP_NET_RAW` capabilities. If running via systemd, run as `root` or grant capabilities explicitly.


Architecture Highlights
---

- **Dual-Path Strategy**: UDP WOL can silently fail when the target is off (ARP resolution fails for powered-down hosts), so raw Ethernet is always sent as a fallback.
- **Smart Interface Discovery**: Sysfs-based interface discovery correctly identifies physical NICs, even when they are enslaved to Linux bridges (e.g., `br0`), avoiding hardcoded interface names like `eth0`.
- **ARP Stimulation**: When monitoring for DHCP machines coming online, the bot sends broadcast pings to stimulate ARP table population.

TODO
---

- Support Remote Shutdown
