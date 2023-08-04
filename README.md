Wakeup Bot - Go Version
===

Description
---

This program run as a Telegram Bot and waiting user to input command to help you start computer remotely:

| Command | Description|
| ------- | ---------- |
| `/wake` | Send Magic packet (WOL) to target Machine |
| `/check` | Check Whether the machine is up or not, by sending `ping` to its IP |
| `/hello` | Just say "Hi", to make sure that bot is running property |

Best fit when you are having a running 24/7 tiny machine likes Raspberry Pi or any embedded Platform which supports Linux and Go.

**Requires Go 1.20**.

**Note**:

It use built-in WOL lib by [mdlayher](https://github.com/mdlayher/wol) and will send WOL packet by using 2 mode:

- UDP Mode: Root not required
- Raw mode: If UDP mode is not working, then it will try to send as Raw Mode. Require Super privileges.

**Configuration**:

See `config.json.template` -> Rename it to `config.json` and edit content, place alongside with the main binary.

| Field | Description                                                                                                                                                                                                              |
| --- |--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `remote_ip` | Format: `ip:port`. Normally the port will be `7` or `9`. See [here](https://www.manageengine.com/products/oputils/tech-topics/what-is-wake-on-lan.html#:~:text=Wake%20On%20LAN%20(WOL)%20is,UDP%20ports%207%20and%209.). |
| `remote_mac` | Format: `XX:XX:XX:XX:XX:XX`. The MAC Address of remote Machine' interface |
| `inet_interface` | Required when Use Raw mode in *nix. It look likes `ensXX` or `ethX` |
| `bot_token` | Telegram bot token |
| `chat_id` | Who's the bot owner? |

**TODO**:

- Support WOL with Password 
- Support Remote Shutdown
- ...


