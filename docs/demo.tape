# VHS script for the README demo GIF.
# Regenerate with:  VHS_NO_SANDBOX=true vhs docs/demo.tape

Output docs/demo.gif

Set Shell bash
Set FontSize 15
Set Width 1200
Set Height 700
Set Padding 12
Set Theme "Catppuccin Mocha"
Set TypingSpeed 55ms

Hide
Type "clear"
Enter
Sleep 1s
Show

# A real article: styled text, colored images, clickable links.
Type "pixsurf en.wikipedia.org/wiki/Cat"
Sleep 700ms
Enter
Sleep 5s

PageDown
Sleep 3s
Down@110ms 10
Sleep 2500ms
PageDown
Sleep 3s

# Jump to Hacker News with the URL bar — the page keeps its own colors.
Type "g"
Sleep 700ms
Type "news.ycombinator.com"
Sleep 500ms
Enter
Sleep 4s
Down@110ms 8
Sleep 2500ms

# Markdown files render too, not as raw text.
Type "g"
Sleep 700ms
Type "raw.githubusercontent.com/golang/go/master/README.md"
Sleep 500ms
Enter
Sleep 4s
Down@110ms 6
Sleep 2500ms

# Back through history.
Type "b"
Sleep 2500ms

Type "q"
Sleep 1s
