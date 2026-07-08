@echo off
setlocal EnableExtensions EnableDelayedExpansion

set "ROOT=%~dp0"
set "COMMAND=%~1"

if "%COMMAND%"=="" goto help
shift /1

set "SCRIPT_ARGS="
:collect_args
if "%~1"=="" goto args_done
set "SCRIPT_ARGS=!SCRIPT_ARGS! %~1"
shift /1
goto collect_args

:args_done
if /I "%COMMAND%"=="setup" goto setup
if /I "%COMMAND%"=="check" goto check
if /I "%COMMAND%"=="test" goto test
if /I "%COMMAND%"=="run" goto run
if /I "%COMMAND%"=="build" goto build
if /I "%COMMAND%"=="package" goto build
if /I "%COMMAND%"=="verify" goto verify
if /I "%COMMAND%"=="release" goto release
if /I "%COMMAND%"=="hook" goto hook
if /I "%COMMAND%"=="frontend-build" goto frontend_build
if /I "%COMMAND%"=="guide" goto guide
if /I "%COMMAND%"=="help" goto help
goto unknown

:setup
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT%scripts\setup.ps1" !SCRIPT_ARGS!
exit /b %ERRORLEVEL%

:check
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT%scripts\check-env.ps1" !SCRIPT_ARGS!
exit /b %ERRORLEVEL%

:test
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT%scripts\test.ps1" !SCRIPT_ARGS!
exit /b %ERRORLEVEL%

:run
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT%scripts\run.ps1" !SCRIPT_ARGS!
exit /b %ERRORLEVEL%

:build
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT%scripts\package.ps1" !SCRIPT_ARGS!
exit /b %ERRORLEVEL%

:verify
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT%scripts\verify.ps1" !SCRIPT_ARGS!
exit /b %ERRORLEVEL%

:release
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT%scripts\release.ps1" !SCRIPT_ARGS!
exit /b %ERRORLEVEL%

:hook
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT%scripts\install-hooks.ps1" !SCRIPT_ARGS!
exit /b %ERRORLEVEL%

:frontend_build
powershell -NoProfile -ExecutionPolicy Bypass -File "%ROOT%scripts\frontend-build.ps1" !SCRIPT_ARGS!
exit /b %ERRORLEVEL%

:guide
start "" "%ROOT%frontend\src\guide.html"
exit /b 0

:unknown
echo Unknown command: %COMMAND%
echo.
call :print_help
exit /b 1

:help
call :print_help
exit /b 0

:print_help
echo SemanticIDF developer commands
echo.
echo   dev setup             Install repo-local Go/Wails runtime and git hook
echo   dev check             Check repo-local runtime
echo   dev test              Run go tests
echo   dev run               Run the Wails app with repo-local Go
echo   dev build             Build the Wails executable
echo   dev verify            Run diff check, tests, and Wails build
echo   dev release           Prepare release metadata from release notes
echo   dev hook              Install the pre-commit hook
echo   dev frontend-build    Validate static frontend files
echo   dev guide             Open the user guide HTML
echo.
echo Extra arguments are forwarded to the target PowerShell script.
exit /b 0
