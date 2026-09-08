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
    │  ├ 0  logs  zsh                                    ~/dotfiles/docs
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
| `space` | mark a row — a session, window or pane — to act on several at once |
| `x` | cut the window or pane under the cursor; on a session name, all of its windows |
| `p` `P` | paste after, or before, the cursor. On a session name it lands at that session's end |
| `J` `K` | move a window within its session, or swap a pane with its neighbour |
| `M` | merge the marked sessions, or the one under the cursor, into the one the popup was opened from |
| `S` | move what is cut into a new session, named as you type it |
| `r` | rename the window or session; on a pane it sets a title, which an empty name takes away |
| `d` | close the pane, window or session under the cursor, once `y` confirms |
| `L` | lay out the panes of the window under the cursor — see below |
| `enter` | go to the row under the cursor and close |
| `q` `esc` | close; `esc` drops the marks first |
| `?` | the full key list |

`space` marks a row and `x`, `d`, `S` and `M` then act on everything marked
instead of on the cursor. Sessions can be marked too, so several go into one
session with `x` and `p`, fold into the one you came from with `M`, or close
together with `d`. A window or pane marked inside a marked session is left out
of the count: the session already takes it.

Panes take part in all of it: cut a pane and paste it onto a window to join it
there as a split, onto a particular pane to split that one, or onto a session
name to break it out into a window of its own. `r` on a pane gives it a name of
its own — tmux calls it the pane title — which the tree shows in place of the
command the pane runs, with the command kept beside it. The name sticks: tmux
stops the program in that pane from retitling it, until an empty name hands the
title back. A title a program set for itself is not a name, and stays out of the
tree, so a shell that titles every prompt does not clutter it.

`d` closes things, and asks first: it names what is about to go and waits for
`y`. The session the popup was opened from is the one thing it will not close,
marked or not.

## Laying out a window

`L` on a window or pane draws that window's panes as boxes, to scale, and lets
you point at them:

```
 work · edit                                          5 panes · 200x50

 ┌───────────────────────┬──────────────────────┐
 │▶ 0  nvim ●            │2  server             │
 │100x25                 │99x25                 │
 │                       │                      │
 ├───────────────────────┼───────────┬──────────┤
 │1  zsh                 │3  logs    │4  psql   │
 │100x24                 │49x24      │49x24     │
 └───────────────────────┴───────────┴──────────┘
```

| Key | Does |
|---|---|
| `h` `j` `k` `l` | point at the pane on that side |
| `H` `J` `K` `L` | push the pane that way, swapping it with the one there |
| `x` | cut the pane under the cursor; `x` again puts it back |
| `p` | place what is cut beside that pane — `h` `j` `k` `l` says which side |
| `s` | resize: `h` `j` `k` `l` nudge the border, `H` `J` `K` `L` by five, `esc` when done |
| `=` | step through the preset layouts — even, main, tiled |
| `z` | zoom the pane, and unzoom it |
| `u` | undo the last change |
| `enter` | go to that pane and close |
| `esc` | back to the tree |

Which pane is beside which comes from the rectangles tmux reports, so `h` and
`l` cross to the pane that is really there rather than to the next one by
number. Resizing is the border you push: `l` moves the pane's right edge right,
whatever side of the window it is on. A whole run of nudges undoes in one `u`,
so it is worth nudging freely.

The clipboard is the same one the tree cuts into, which is how a pane crosses
windows and lands where you want it: cut it here or in the tree, open the other
window's map, then `p` and the side it belongs on. `u` puts back changes to this
window's own layout — a size, a push, a preset — rather than a pane that came
from somewhere else.

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

The version the binary was built at sits in the bottom-right corner of the
popup, so an install or an update can be told apart from the one before it. A
binary built by hand rather than from a release says `dev`.

Requires tmux 3.2 or newer, for `display-popup`. Naming a pane needs 3.5, for
the `allow-set-title` option that holds the name against the program running
under it: on anything older `r` on a pane says so and leaves it alone, panes
read as the command they run, and everything else works as it does here.

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
`break-pane`, `swap-window`, `swap-pane`, `kill-window`, `kill-pane`,
`kill-session`, `resize-pane`, `select-layout`. Sessions, windows and panes are
named to tmux by id (`$1`, `@4`, `%9`), never by name, so a rename between
reading the tree and moving something cannot misdirect it.

Undo in the layout view is tmux's own doing: `#{window_layout}` writes a
window's geometry down as a string that `select-layout` takes back, so a resize
or a preset undoes exactly. A push undoes as the opposite push, since a layout
string carries sizes rather than which pane sits where.

## Development

```sh
task check   # build, vet, and the tests
```

The tests drive a real tmux: each one starts a throwaway server on a socket of
its own, presses keys at the model, and asserts on the layout tmux reports back.

Releasing is one task:

```sh
task release -- 0.2.0
```

It runs the checks, writes `VERSION`, commits it, annotates the tag, and prints
the `git push` line to run — pushing the tag is what starts the release
workflow, which builds the assets with goreleaser. The tag and `VERSION` have
to agree, because every clone asks for the asset `VERSION` names; the workflow
refuses a tag that says otherwise.
