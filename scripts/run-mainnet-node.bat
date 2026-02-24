@echo off
REM DevInsiderCoin — Start Mainnet Node (Windows)
cd /d "%~dp0\.."

go build -o dvcnode.exe ./cmd/dvcnode

dvcnode.exe --network mainnet --datadir .\data\mainnet %*
