```shell
#!/bin/bash

# Create 4 HTML files
touch file1.html
touch file2.html
touch file3.html
touch file4.html

# Move the files to the web folder
mkdir web
mv file1.html web
mv file2.html web
mv file3.html web
mv file4.html web

# Change directory to the web folder
cd web
```