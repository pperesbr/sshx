<p align="center">
  <img src="assets/logo.svg" alt="sshx" width="480">
</p>

# sshx

Go SSH client library for network equipment automation.

Born from a real need: accessing routers and olts in a telecom transport
and access network environment. The equipment is modern, the ciphers are modern,
and we don't need legacy compatibility hacks. sshx only supports equipment that
speaks modern cryptography — no CBC, no DES, no RC4.

No regex. No legacy cipher hacks. Pure Go.

---

## Install

```bash
go get github.com/pperesbr/sshx
```

Requires Go 1.24+.

## Usage

### Huawei NE40 (VRP)

```go
sess, err := sshx.NewSession(sshx.Config{
    Host:           "huawei-router",
    Username:       "operator",
    Password:       "secret",
    Prompt:         "<huawei-router>",
    ConnectTimeout: 10 * time.Second,
    CommandTimeout: 30 * time.Second,
})
if err != nil {
    log.Fatal(err)
}
defer sess.Close()

ctx := context.Background()

sess.Run(ctx, "screen-length 0 temporary")
output, err := sess.Run(ctx, "display bgp peer")
```

### Cisco ASR 9000 (IOS-XR)

```go
sess, err := sshx.NewSession(sshx.Config{
    Host:     "cisco-router",
    Username: "operator",
    Password: "secret",
    Prompt:   "RP/0/RSP0/CPU0:cisco-router#",
})
defer sess.Close()

sess.Run(ctx, "terminal length 0")
output, _ := sess.Run(ctx, "show bgp summary")
```

### Nokia SR 7750 (SR OS)

```go
sess, err := sshx.NewSession(sshx.Config{
    Host:     "nokia-router",
    Username: "operator",
    Password: "secret",
    Prompt:   "A:nokia-router#",
})
defer sess.Close()

sess.Run(ctx, "environment no more")
output, _ := sess.Run(ctx, "show router bgp summary")
```

### Juniper MX (Junos)

```go
sess, err := sshx.NewSession(sshx.Config{
    Host:     "juniper-router",
    Username: "operator",
    Password: "secret",
    Prompt:   "operator@juniper-router-re0>",
})
defer sess.Close()

sess.Run(ctx, "set cli screen-length 0")
output, _ := sess.Run(ctx, "show bgp summary")
```

### Nokia ISAM OLT (7360/7302) — prompt changes between contexts

```go
sess, err := sshx.NewSession(sshx.Config{
    Host:     "nokia-olt",
    Username: "operator",
    Password: "secret",
    Prompt:   "nokia-olt:operator>#",
})
defer sess.Close()

// Prompt changes when entering environment context
sess.SetPrompt("nokia-olt:operator>environment#")
sess.Run(ctx, "environment print no-more")

// Back to normal prompt
sess.SetPrompt("nokia-olt:operator>#")
sess.Run(ctx, "exit all")

output, _ := sess.Run(ctx, "show service service-using")
```

### Huawei MA5800 OLT

```go
sess, err := sshx.NewSession(sshx.Config{
    Host:     "huawei-olt",
    Username: "operator",
    Password: "secret",
    Prompt:   "huawei-olt>",
})
defer sess.Close()

sess.Run(ctx, "undo smart")
sess.Run(ctx, "scroll")
output, _ := sess.Run(ctx, "display ip routing-table")
```

## Config

| Field            | Type            | Default    | Description                                   |
|------------------|-----------------|------------|-----------------------------------------------|
| `Host`           | `string`        | —          | Required. Hostname or IP                      |
| `Port`           | `int`           | `22`       | SSH port                                      |
| `Username`       | `string`        | —          | Required. SSH username                        |
| `Password`       | `string`        | —          | Required. SSH password                        |
| `Prompt`         | `string`        | —          | Required. Device prompt to detect             |
| `ConnectTimeout` | `time.Duration` | `10s`      | TCP + SSH handshake timeout                   |
| `CommandTimeout` | `time.Duration` | `30s`      | Per-command timeout (when no context deadline) |
| `Ciphers`        | `[]string`      | see below  | Ciphers to offer, in preference order         |

### Default ciphers

1. `aes256-gcm@openssh.com`
2. `aes128-gcm@openssh.com`
3. `chacha20-poly1305@openssh.com`
4. `aes256-ctr`
5. `aes192-ctr`
6. `aes128-ctr`

## API

```go
sess, err := sshx.NewSession(cfg)    // Connect, consume login banner
output, err := sess.Run(ctx, cmd)    // Send command, wait for prompt
err := sess.Write(cmd)               // Send command, don't wait
sess.SetPrompt(newPrompt)            // Change expected prompt
sess.Close()                         // Close session and connection
```

---

## How It Works — Prompt Detection in Detail

### The Problem

When you send a command via SSH to a router, the device sends bytes back.
You need to know **when the device finished responding**. The way to know is:
the **prompt** appeared at the end of the output.

But there are two problems:

1. The bytes come mixed with **ANSI escape sequences** (terminal control codes)
   that break string matching.
2. The bytes arrive in **chunks of variable size** over the network — the prompt
   can be split across two or more chunks.

sshx solves both problems with two techniques working together:

```
Bytes from SSH
     │
     ▼
┌─────────────┐     Filters out ANSI escape sequences.
│  ANSI FSM   │     Only clean, visible bytes pass through.
│  (4 states) │     No regex, no string operations.
└──────┬──────┘
       │ clean bytes only
       ▼
┌─────────────┐     Compares the last N bytes against
│  Circular   │     the expected prompt.
│   Buffer    │     Fixed memory, no growing buffers.
└──────┬──────┘
       │
       ▼
  Prompt found? ──yes──▶ Return output
       │
       no
       │
       ▼
  Read next chunk
```

---

### Why FSM Instead of Regex

A common approach to strip ANSI escape sequences is a regex like:

```
\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*\x07
```

This works, but has concrete problems compared to a byte-level FSM:

#### 1. Regex needs a complete buffer to scan

Regex operates on a string or byte slice. You need to accumulate bytes into a
buffer first, then run the regex on the entire thing. Every time a new chunk
arrives from SSH, you either re-scan the whole buffer (O(n) growing cost) or
you need to track where you left off (complex, error-prone).

The FSM processes each byte exactly once, as it arrives. No accumulation, no
re-scanning. O(1) per byte, always.

```
Regex approach:
  chunk 1 arrives → scan 4096 bytes
  chunk 2 arrives → scan 8192 bytes (re-scanning chunk 1)
  chunk 3 arrives → scan 12288 bytes (re-scanning 1 and 2)
  ...growing cost with each chunk

FSM approach:
  chunk 1 arrives → process 4096 bytes
  chunk 2 arrives → process 4096 bytes (only the new ones)
  chunk 3 arrives → process 4096 bytes (only the new ones)
  ...constant cost per chunk
```

#### 2. Regex allocates memory on every call

Go's `regexp.ReplaceAll` allocates a new byte slice for the result every time
it's called. In a hot path — processing output from 200 concurrent SSH sessions —
this creates pressure on the garbage collector.

The FSM allocates zero memory during processing. The circular buffer is allocated
once at creation and reused for every byte.

```
Regex: regexp.ReplaceAll(buffer, nil)
  → allocates new []byte for result
  → old buffer becomes garbage
  → GC has to clean it up
  → repeat for every chunk

FSM: r.feedByte(b)
  → one switch statement
  → writes to pre-allocated circular buffer
  → zero allocations
  → GC has nothing to do
```

#### 3. Regex can miss edge cases

The pattern `\x1b\[[0-9;]*[a-zA-Z]` covers CSI sequences, but ANSI has more
types: OSC (Operating System Command), two-byte escapes, charset designations.
To cover them all, the regex grows complex and fragile:

```
\x1b\[[0-9;]*[a-zA-Z]     ← CSI only
|\x1b\][^\x07]*\x07        ← OSC
|\x1b[()][0-9A-B]          ← charset
|\x1b[\?]?[0-9;]*[hl]      ← private modes
```

Each new pattern risks breaking existing ones. The FSM handles all of these
with 4 states and a switch — the logic is explicit and each byte's handling
is visible:

```go
case stateEsc:
    switch b {
    case '[':  r.state = stateCSI    // CSI
    case ']':  r.state = stateOSC    // OSC
    default:   r.state = stateNormal // two-byte escape, charset, etc.
    }
```

#### 4. Regex can't process bytes inline with prompt detection

With regex, you need two separate passes: first strip ANSI, then search for the
prompt. With the FSM, both happen in the same byte loop — the FSM filters and
the circular buffer checks, for each byte, in one pass:

```go
// One function call per byte does everything:
// 1. Saves raw byte
// 2. Filters ANSI (FSM)
// 3. Pushes clean byte to circular buffer
// 4. Checks for prompt match
func (r *Reader) feedByte(b byte) bool { ... }
```

#### Summary

| Aspect                | Regex                      | FSM                        |
|-----------------------|----------------------------|----------------------------|
| Cost per chunk        | O(total bytes so far)      | O(chunk size only)         |
| Memory allocations    | Every call                 | Zero                       |
| ANSI coverage         | Pattern-dependent          | Explicit per byte          |
| Inline prompt check   | Separate pass              | Same loop                  |
| Code complexity       | One-liner but opaque       | More code but transparent  |
| Debuggability         | Hard to trace which part matched | Step through byte by byte |

---

### Part 1: ANSI Escape Sequences

#### What are they

When the router sends `<ROUTER-PE01>`, the bytes that actually arrive on the
wire can look like this:

```
What you expect:  < R O U T E R - P E 0 1 >
What arrives:     \x1b [ 4 2 D < R O U T E R - P E 0 1 >
                  ^^^^^^^^^^^
                  ANSI garbage — "move cursor 42 positions back"
```

The `\x1b` is a **single byte** (value 0x1b = 27 decimal = ESC in ASCII).
It's not four characters `\`, `x`, `1`, `b` — it's one byte that we *write*
as `\x1b` because it has no visible representation.

If we don't filter these out, our prompt matching will fail because the buffer
contains invisible bytes mixed with the real text.

#### Types of escape sequences

```
A byte arrives from the router
│
├── Is it 0x1b (ESC)?
│   │
│   │  The NEXT byte determines the type:
│   │
│   ├── '['  →  CSI (Control Sequence Introducer)
│   │           Variable length: \x1b[ + parameters + final letter
│   │           Examples:
│   │             \x1b[42D     → move cursor back (common on Huawei VRP)
│   │             \x1b[0;32m   → set text color to green
│   │             \x1b[0m      → reset all formatting
│   │
│   ├── ']'  →  OSC (Operating System Command)
│   │           Runs until BEL byte (0x07): \x1b] + text + \x07
│   │           Example:
│   │             \x1b]0;ROUTER-PE01\x07  → set terminal window title
│   │           Danger: the hostname appears inside! Must be consumed.
│   │
│   └── anything else  →  Two-byte escape (ends immediately)
│                          Examples:
│                            \x1bM  → cursor up (reverse index)
│                            \x1bD  → cursor down (index)
│                            \x1b7  → save cursor position
│
└── NOT 0x1b
    └── Normal visible byte → goes to the circular buffer
```

#### The State Machine

The FSM has 4 states. Each byte causes exactly one state transition:

```
                    ┌──────────────────────────────────────┐
                    │                                      │
                    ▼                                      │
              ┌──────────┐                                 │
         ┌───▶│  NORMAL  │◀──── letter (0x40-0x7E) ───────┤
         │    └────┬─────┘                                 │
         │         │                                       │
         │     0x1b (ESC)                                  │
         │         │                                       │
         │         ▼                                       │
         │    ┌──────────┐                                 │
         │    │  ESCAPE   │                                │
         │    └──┬───┬───┘                                 │
         │       │   │                                     │
         │    '['│   │']'                                  │
         │       │   │                            ┌────────┴────────┐
         │       ▼   └──────────────────────────▶ │      OSC        │
         │  ┌─────────┐                           │ consumes until  │
         │  │   CSI   │                           │ BEL (0x07)      │
         │  │ params  │                           └─────────────────┘
         │  └─────────┘
         │
         └─── any other byte: two-byte escape, done
```

Concrete example, byte by byte, with `\x1b[42D<ROUTER>`:

```
Byte:   \x1b   [     4     2     D     <     R     O     U     T     E     R     >
State:  N→ESC  →CSI  CSI   CSI   →N    N     N     N     N     N     N     N     N
                ^^^^^^^^^^^^^^^^^^^
                All consumed, never reach the buffer

Clean bytes that reach the buffer:          <  R  O  U  T  E  R  >
```

---

### Part 2: Circular Buffer (Sliding Window)

#### The concept

After ANSI filtering, we have a stream of clean bytes. We need to detect when
the last N bytes (where N = length of the prompt) match the expected prompt.

We use a **fixed-size array** where a pointer wraps around when it reaches the
end, overwriting the oldest byte. This is called a circular buffer.

#### The two controls

**`pos`** — where the next byte will be written. Starts at 0, advances by 1
after each write. When it reaches the end of the array, it wraps back to 0.

**`filled`** — how many bytes have been written total (max = array size).
Exists only to prevent comparing garbage during startup, before the buffer
has enough bytes.

#### Step-by-step example

Prompt: `<PE01>` (6 bytes). Array has 6 positions.

```
Initial state:
┌───┬───┬───┬───┬───┬───┐
│ _ │ _ │ _ │ _ │ _ │ _ │
└───┴───┴───┴───┴───┴───┘
  0   1   2   3   4   5
  ▲pos                      filled=0
                            0 < 6 → don't compare yet

Byte '<' arrives → write at pos=0, advance pos to 1:
┌───┬───┬───┬───┬───┬───┐
│ < │ _ │ _ │ _ │ _ │ _ │
└───┴───┴───┴───┴───┴───┘
  0   1   2   3   4   5
      ▲pos                  filled=1
                            1 < 6 → don't compare

Byte 'P' arrives → write at pos=1, advance pos to 2:
┌───┬───┬───┬───┬───┬───┐
│ < │ P │ _ │ _ │ _ │ _ │
└───┴───┴───┴───┴───┴───┘
  0   1   2   3   4   5
          ▲pos              filled=2

... bytes 'E', '0', '1' arrive ...

Byte '>' arrives → write at pos=5, advance pos to 0 (WRAPPED):
┌───┬───┬───┬───┬───┬───┐
│ < │ P │ E │ 0 │ 1 │ > │
└───┴───┴───┴───┴───┴───┘
  0   1   2   3   4   5
  ▲pos (back to 0)         filled=6

  filled(6) == 6 → NOW COMPARE!
  Read from pos=0: <, P, E, 0, 1, >
  Compare with:    <, P, E, 0, 1, >
  ✅ MATCH — prompt found!
```

#### When bytes are "out of order" in memory

After a lot of output, the pointer has wrapped around multiple times.
The array in memory looks disordered, but reading from `pos` gives the
correct chronological order.

```
After many bytes, 'G' arrives at pos=0 (overwrites '<'):
┌───┬───┬───┬───┬───┬───┐
│ G │ P │ E │ 0 │ 1 │ > │
└───┴───┴───┴───┴───┴───┘
  0   1   2   3   4   5
      ▲pos                  filled=6

  Read from pos=1: P, E, 0, 1, >, G
  Compare with:    <, P, E, 0, 1, >
  P ≠ < → no match ❌

More bytes arrive... now the buffer has:
┌───┬───┬───┬───┬───┬───┐
│ 0 │ 1 │ > │ < │ P │ E │
└───┴───┴───┴───┴───┴───┘
  0   1   2   3   4   5
              ▲pos=3        filled=6

  Read from pos=3: <, P, E, 0, 1, >
                   (wraps: pos 3→4→5→0→1→2)
  Compare with:    <, P, E, 0, 1, >
  ✅ MATCH!
```

The reading always starts at `pos` (the oldest byte, about to be overwritten)
and ends at `pos - 1` (the most recent byte). One full loop around the buffer.

#### The wrap-around with modulo

```go
pos = (pos + 1) % len(window)
```

The `%` operator returns the remainder of division. With `len(window) = 6`:

```
pos=0 → (0+1) % 6 = 1
pos=1 → (1+1) % 6 = 2
pos=4 → (4+1) % 6 = 5
pos=5 → (5+1) % 6 = 0  ← wrapped back to start
pos=0 → (0+1) % 6 = 1  ← cycle continues
```

Works like a clock. After 11 comes 0:

```
(10 + 1) % 12 = 11
(11 + 1) % 12 = 0   ← wrapped
(0  + 1) % 12 = 1
```

---

### Part 3: Goroutines and Timeout

The reading happens in a background goroutine:

```
MAIN GOROUTINE (called WaitForPrompt):
│
├── creates channels: done, readErr
├── spawns reading goroutine ──────▶  READING GOROUTINE:
│                                     │
│                                     ├── Read(buf) in 4096-byte chunks
│                                     ├── feedByte each byte
│                                     │   ├── ANSI? consume, skip
│                                     │   └── clean? push to buffer → compare
│                                     ├── prompt found? close(done)
│                                     └── error? readErr <- err
│
├── select {
│     case <-done:        prompt found, return output
│     case err := <-readErr: read error, return partial output
│     case <-ctx.Done():  timeout or cancellation
│   }
└── return
```

The `readErr` channel has a **buffer of 1** (`make(chan error, 1)`) to prevent
a zombie goroutine. If the context times out and the main goroutine returns,
the reading goroutine needs to send its error somewhere. Without a buffer, it
would block forever on `readErr <- err`. With a buffer of 1, it drops the error
in the buffer and terminates cleanly.

---

### Trade-offs

#### Prompt must be known in advance

sshx requires you to set the exact prompt string. It does **not** auto-detect
the prompt by looking for `#` or `>` patterns.

**Why:** Auto-detection fails when the output contains the prompt string.
For example, `display current-configuration` on Huawei VRP may contain:

```
header login information "Welcome to <ROUTER-PE01>"
```

An auto-detector would stop reading there, losing the rest of the config.
By requiring the exact prompt, we trade convenience for correctness.

#### First occurrence wins

If the exact prompt string appears in the middle of the output (inside a banner,
a remark, or a config block), sshx will match it and stop reading. In practice
this is rarely a problem because prompts are typically hostnames that don't appear
verbatim in config output.

#### Fixed memory per command

The circular buffer uses exactly `len(prompt)` bytes, regardless of how much
output the device sends. A 10MB routing table uses the same buffer memory as
a 10-byte response. The full raw output is accumulated separately in a
`bytes.Buffer` for the caller to use.

---

## Supported Equipment

sshx uses only modern ciphers supported by Go's `crypto/ssh`. If the equipment
offers at least one cipher from the accepted list, it works.

### Accepted Ciphers

| Cipher                             | Type     |
|------------------------------------|----------|
| `aes256-gcm@openssh.com`           | AEAD     |
| `aes128-gcm@openssh.com`           | AEAD     |
| `chacha20-poly1305@openssh.com`    | AEAD     |
| `aes256-ctr`                       | Stream   |
| `aes192-ctr`                       | Stream   |
| `aes128-ctr`                       | Stream   |

### Rejected Ciphers (never used, even if offered)

`aes256-cbc`, `aes192-cbc`, `aes128-cbc`, `3des-cbc`, `des-cbc`, `blowfish-cbc`, `cast128-cbc`, `arcfour*`, `rijndael-cbc@lysator.liu.se`, `AEAD_AES_*_GCM` (Huawei proprietary naming)

### Routers

| Vendor  | Model    | Platform | Supported | Common Ciphers                                                                  |
|---------|----------|----------|-----------|---------------------------------------------------------------------------------|
| Huawei  | NE40     | VRP      | ✅        | aes256-gcm, aes128-gcm, aes256-ctr, aes192-ctr, aes128-ctr                     |
| Cisco   | ASR 9000 | IOS-XR   | ✅        | aes256-gcm, aes128-gcm, chacha20-poly1305, aes256-ctr, aes192-ctr, aes128-ctr  |
| Nokia   | SR 7750  | SR OS    | ✅        | aes256-ctr, aes192-ctr, aes128-ctr                                              |
| Juniper | MX       | Junos    | ✅        | chacha20-poly1305, aes256-gcm, aes128-gcm, aes256-ctr, aes192-ctr, aes128-ctr  |

### OLTs

| Vendor  | Model         | Variant | Supported | Common Ciphers                                              |
|---------|---------------|---------|-----------|-------------------------------------------------------------|
| Huawei  | MA5800        | X7      | ✅        | aes256-gcm, aes128-gcm, aes256-ctr, aes192-ctr, aes128-ctr |
| Huawei  | MA5800        | X17     | ✅        | aes256-gcm, aes128-gcm, aes256-ctr, aes192-ctr, aes128-ctr |
| Nokia   | 7360          | FX-8    | ✅        | aes256-ctr, aes192-ctr, aes128-ctr                          |
| Nokia   | 7360          | FX-16   | ✅        | aes256-ctr, aes192-ctr, aes128-ctr                          |
| Nokia   | 7302          | FX-16   | ✅        | aes256-ctr, aes192-ctr, aes128-ctr                          |
| Nokia   | 7342          | FX      | ❌        | None — only offers CBC ciphers                               |

> **Nokia SR 7750 / 7360 / 7302**: No AEAD ciphers available. Connections use
> CTR mode with separate MAC (hmac-sha2-256 or hmac-sha2-512). Secure but less
> optimal than AEAD.

> **Nokia 7342 FX**: **NOT COMPATIBLE**. Only offers `aes128-cbc`, `blowfish-cbc`,
> `3des-cbc`, `des-cbc` — all insecure. sshx will refuse to connect.
> This equipment needs a firmware upgrade to support CTR or GCM ciphers.

### How to Check Your Equipment

```bash
ssh -vvv <hostname> 2>&1 | grep "peer server"
```

If the output includes any cipher from the **Accepted** list, the equipment
is compatible with sshx.

## Running Tests

Tests connect to real equipment. No mocks.

```bash
# Using environment variables
SSHX_HOST=huawei-router \
SSHX_USER=operator \
SSHX_PASS=secret \
SSHX_PROMPT="<huawei-router>" \
go test -v -count=1 -run TestHuaweiVRP ./...

# Using the test script (prompts for credentials)
./test.sh huawei-vrp huawei-router "<huawei-router>"
./test.sh cisco-iosxr cisco-router "RP/0/RSP0/CPU0:cisco-router#"
./test.sh nokia-sros nokia-router "A:nokia-router#"
./test.sh juniper juniper-router "operator@juniper-router-re0>"
./test.sh huawei-olt huawei-olt "huawei-olt>"
./test.sh nokia-7360 nokia-olt "nokia-olt:operator>#"
```

Tests skip automatically when `SSHX_HOST` is not set.
