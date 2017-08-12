@echo off
SET mypath=%~dp0
SET depth=2
pushd %mypath%
echo Working dir: %CD%
echo Depth: %depth%
go run .\vendor\github.com\smartystreets\goconvey\goconvey.go -depth %depth%
