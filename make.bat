@echo off
REM Wrapper para permitir digitar "make <comando>" no Prompt de Comando do Windows (cmd.exe)
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0make.ps1" %*

