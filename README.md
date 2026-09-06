# tmux-arrange

Rearrange tmux sessions, windows and panes from a tree in a popup: cut a window
where it is, put the cursor where it belongs, paste it there.

```
    ▾ notes                                                    2 windows
    │    0  read ●                                                     ~
    │    1  write                                        ~/dotfiles/link
    ▾ work ●                                            here · 2 windows
    │    0  edit ●                                            ~/dotfiles
▶ ✂ │ ▾2 1  server                                      ~/dotfiles/tools
    │  ├ 0  zsh                                          ~/dotfiles/docs
    │  └ 1  nvim                                        ~/dotfiles/tools
```

`tmux` can already move a window with `swap-window` and `move-window`, and join
a pane with `join-pane`, but each of those wants you to name the target from
memory. This shows you every session, window and pane at once and lets you point
at the place instead.

## Keys

| Key | Does |
|---|---|
| `j` `k` `g` `G` | move the cursor |
| `h` `l` | fold and unfold — `l` on a window shows its panes |
| `space` | mark a row, to act on several at once |
| `x` | cut the window or pane under the cursor; on a session name, all of its windows |
| `p` `P` | paste after, or before, the cursor. On a session name it lands at that session's end |
| `J` `K` | move a window within its session, or swap a pane with its neighbour |
| `M` | merge the session under the cursor into the one the popup was opened from |
| `S` | move what is cut into a new session, named as you type it |
| `r` | rename the window or session |
| `enter` | go to the row under the cursor and close |
| `q` `esc` | close; `esc` drops the marks first |
| `?` | the full key list |

Panes take part in all of it: cut a pane and paste it onto a window to join it
there as a split, onto a particular pane to split that one, or onto a session
name to break it out into a window of its own.

## Install

With [tpm](https://github.com/tmux-plugins/tpm), add to `~/.tmux.conf`:

```tmux
set -g @plugin 'hpcsc/tmux-arrange'
```

then press `prefix + I`. `prefix + W` opens the tree.

The plugin is a Go program, so something has to put a binary next to the
checkout. On the first tmux start after installing, it downloads the release
build for your platform (`darwin`/`linux`, `amd64`/`arm64`) and checks it
against the release's `checksums.txt`. If there is no asset for your platform,
or no network, it builds from the source tpm already cloned, which needs a Go
toolchain. Either way the result lands in `bin/tmux-arrange`, and what happened
is written to `install.log` beside it.

Requires tmux 3.2 or newer, for `display-popup`.

## Options

| Option | Default | |
|---|---|---|
| `@arrange-key` | `W` | the key, bound under the prefix |
| `@arrange-width` | `80%` | popup width |
| `@arrange-height` | `80%` | popup height |
| `@arrange-title` | ` arrange ` | popup title |

```tmux
set -g @arrange-key 'w'
set -g @arrange-height '90%'
```

## How it hangs together

`tmux-arrange.tmux` binds the key to `scripts/arrange.sh`, through `run-shell`
rather than `display-popup`: `run-shell` expands `#{client_name}`, and the popup
has to be told which client to switch when you press `enter`, because a popup's
own tty tells tmux nothing about the client that opened it.

Everything the tool does is a tmux command — `move-window`, `join-pane`,
`break-pane`, `swap-window`, `swap-pane`. Sessions, windows and panes are named
to tmux by id (`$1`, `@4`, `%9`), never by name, so a rename between reading the
tree and moving something cannot misdirect it.

## Development

```sh
task check   # build, vet, and the tests
```

The tests drive a real tmux: each one starts a throwaway server on a socket of
its own, presses keys at the model, and asserts on the layout tmux reports back.

Releasing is a tag: bump `VERSION`, tag `v<that>`, and the release workflow
builds the assets with goreleaser. The tag and `VERSION` have to agree — a
clone asks for the asset `VERSION` names.
