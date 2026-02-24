@echo off
REM DevInsiderCoin — Start Testnet Node (Windows)
cd /d "%~dp0\.."

go build -o dvcnode.exe ./cmd/dvcnode

dvcnode.exe --network testnet --datadir .\data\testnet %*
