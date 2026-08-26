@echo off
setlocal
set "WORKDIR=%CD%"
if "%~1"=="--prefix" (
  set "WORKDIR=%~f2"
  shift
  shift
)
if "%~1"=="install" exit /b 0
if "%~1"=="ci" exit /b 0
if "%~1"=="run" if "%~2"=="build" goto build
echo local npm shim supports install, ci, and run build
exit /b 1

:build
if not exist "%WORKDIR%\src\index.html" (
  echo frontend source not found in %WORKDIR%\src
  exit /b 1
)
if not exist "%WORKDIR%\dist" mkdir "%WORKDIR%\dist"
copy /Y "%WORKDIR%\src\index.html" "%WORKDIR%\dist\index.html" >nul
copy /Y "%WORKDIR%\src\app.js" "%WORKDIR%\dist\app.js" >nul
copy /Y "%WORKDIR%\src\styles.css" "%WORKDIR%\dist\styles.css" >nul
exit /b 0
