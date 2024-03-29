# horcrux

Split your file into encrypted horcruxes so that you don't need to remember a passcode

![](https://i.imgur.com/TijN4YP.png)


## How it works

`horcrux` has two commands, `split` and `bind`.

### Splitting

If I have a file called `diary.txt` in my current directory I can call 
```
horcrux split diary.txt
```
and it will prompt me for how many horcruxes I want, and how many will be needed to resurrect the original file. For example I might want 5 horcruxes with the ability to resurrect the file if I have any 3. The horcrux files will be created like so:
```
diary_1_of_5.horcrux
diary_2_of_5.horcrux
...
```
Now you just need to disperse the horcruxes around the house on various USBs or online locations and hope you can recall where they all are!

### Binding

To bind the horcruxes back into the original file just call
```
horcrux bind
```
in the directory containing the horcruxes (or pass the directory as an argument).

## Installation

via homebrew:
```
brew install jesseduffield/horcrux/horcrux
```

via [scoop](https://scoop.sh/):
```
scoop bucket add extras; scoop install horcrux
```

via [binary release](https://github.com/jesseduffield/horcrux/releases)

Example to install the latest release on Linux to `/usr/local/bin` (needs [jq](https://stedolan.github.io/jq/)):

```bash
curl https://api.github.com/repos/jesseduffield/horcrux/releases/latest | \
jq '.assets[] | select(.browser_download_url | endswith("_Linux_x86_64.tar.gz")) | .browser_download_url' | \
xargs curl -Lo /tmp/horcrux.tar.gz && \
tar xfz /tmp/horcrux.tar.gz -C /tmp && \
rm /tmp/horcrux.tar.gz && \
sudo mv /tmp/horcrux /usr/local/bin/horcrux
```

## Who this is for:
* People who need to encrypt a big sensitive file like a diary and don't expect to remember any passwords years from now (but who paradoxically will be capable of remembering where they've hidden their horcruxes)
* People who want to transmit files across multiple channels to substantially reduce the ability for an attacker to intercept
* People named Tom Riddle

## FAQ
Q) This isn't really in line with how horcruxes work in the harry potter universe!

A) It's pretty close! You can't allow any one horcrux to be used to resurrect the original file (and why would you that would be useless) but you can allow two horcruxes to do it (so only off by one). Checkmate HP fans.

Q) How does this work?

A) This uses the [Shamir Secret Sharing Scheme](https://en.wikipedia.org/wiki/Shamir%27s_Secret_Sharing) to break an encryption key into parts that can be recombined to create the original key, but only requiring a certain threshold to do so. I've adapted Hashicorp's implementation from their [vault repo](https://github.com/hashicorp/vault)

Q) How is the key generated

A) Using the Go stdlib's crypto/rand `Read` function

## You can help!

I don't have much time to work on Horcrux but I'm happy to review PRs. One issue you may want to tackle is https://github.com/jesseduffield/horcrux/issues/4 which relates to data integrity.

## Alternatives

* [ssss](http://point-at-infinity.org/ssss/). Works for keys but (as far as I know) not files themselves.
* [horcrux](https://github.com/kndyry/horcrux). Looks like somebody beat me to both the name and concept, however this repo doesn't support thresholds of horcruxes
* [Haystack](https://github.com/henrysdev/Haystack). Implements another file sharding and reassembly algorithm inspired by SSSS, but requires a password for reassembly and does not support thresholds of horcruxes.

## Plugs

Also check out my other projects:
* [My hilarious blog](https://jesseduffield.com/)
* [The _OK?_ Programming Language](https://github.com/jesseduffield/ok)
* [Lazygit](https://github.com/jesseduffield/lazygit)
* [Lazydocker](https://github.com/jesseduffield/lazydocker)
