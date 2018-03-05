@ECHO OFF
SETLOCAL enabledelayedexpansion

SET package=%1
IF [%package%] == [] (
    ECHO usage: %0 ^<package-name^>
)

SET oses=windows linux darwin
SET arch=amd64

(FOR %%o in (%oses%) DO (
    SET GOOS=%%o
    SET GOARCH=%arch%
    SET output_name=gotit-%%o-%arch%
    if "windows"=="%%o" (SET "output_name=!output_name!.exe")

    go build -i -o !output_name! %package%
))
