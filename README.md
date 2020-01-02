# horcrux

Split your file into encrypted horcruxes so that you don't need to remember a passcode

![](https://i.imgur.com/k48QVkF.png)


## How it works

`horcrux` has two commands, `split` and `bind`.

### Splitting

If I have a file called `diary.txt` in my current directory I can call 
```
horcrux split diary.txt
```
and it will prompt me for how many horcruxes I want. If I want 5 horcruxes, it will encrypt my `diary.txt` file with 5 different secret keys, and then split the encrypted result into 5 equal parts to be stored in `.horcrux` files along with the 5 secret keys. This means that you will need all five horcruxes to put the thing back together again and decrypt it. The horcrux files will be created like so:
```
diary_1_of_5.horcrux
diary_2_of_5.horcrux
...
```

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

via binary release 
```

```

## Who this is for:
* People who need to encrypt a big sensitive file like a diary and don't expect to remember any passwords years from now (but who paradoxically will be capable of remembering where they've hidden each horcrux)
* People named Tom Riddle

I have no idea if this program actually has practical use but it's a fun concept that I wanted to implement.
I am aware this isn't quite 1:1 with how horcruxes work in the Harry Potter universe but I think it's close enough.
