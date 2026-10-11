@echo off
rem Prova dell'agente sul PC Windows, senza la pagina di Family Manager.
rem Da tenere nella stessa cartella in cui si vogliono le facciate: scrive li' risultato.txt
rem e, se nel caricatore ci sono fogli, pagina-1.jpg, pagina-2.jpg...
rem L'agente va aperto almeno una volta prima ("Family Manager Agent.exe").
setlocal enabledelayedexpansion
cd /d "%~dp0"

set AGENT=http://127.0.0.1:47115
rem Il sito per conto del quale si chiama: quello del Directus locale. La prima volta l'agente
rem chiede se permetterlo, e si risponde di si'.
set SITE=Origin: http://localhost:8056
set OUT=risultato.txt
set TMPFILE=%TEMP%\fm-agent-prova.txt

echo Family Manager Agent - prova su Windows> "%OUT%"
echo %DATE% %TIME%>> "%OUT%"
ver>> "%OUT%"

echo Avvio l'agente con familymanageragent://open ...
start "" "familymanageragent://open"
timeout /t 3 /nobreak >nul

:status
curl.exe -s -H "%SITE%" %AGENT%/status > "%TMPFILE%" 2>nul
if errorlevel 1 (
    echo L'agente non risponde: aprire "Family Manager Agent.exe" e riprovare.
    echo L'agente non risponde.>> "%OUT%"
    goto log
)
findstr /c:"origin_pending" "%TMPFILE%" >nul
if not errorlevel 1 (
    echo Rispondere alla domanda dell'agente: "Permetti a http://localhost:8056 di usare lo scanner?"
    timeout /t 2 /nobreak >nul
    goto status
)
echo --- status>> "%OUT%"
type "%TMPFILE%">> "%OUT%"
echo.>> "%OUT%"
type "%TMPFILE%"
echo.

findstr /c:"\"scanner\":\"ready\"" "%TMPFILE%" >nul
if errorlevel 1 (
    echo Lo scanner non e' pronto: nessuna scansione.
    echo Nessuna scansione: lo scanner non e' pronto.>> "%OUT%"
    goto log
)

echo Faccio passare i fogli...
del /q pagina-*.jpg 2>nul
curl.exe -s -X DELETE -H "%SITE%" %AGENT%/pages >nul
echo --- scan>> "%OUT%"
curl.exe -s -X POST -H "%SITE%" %AGENT%/scan>> "%OUT%"
echo.>> "%OUT%"

:pages
timeout /t 1 /nobreak >nul
curl.exe -s -H "%SITE%" %AGENT%/pages > "%TMPFILE%"
findstr /c:"\"state\":\"scanning\"" "%TMPFILE%" >nul
if not errorlevel 1 goto pages
echo --- pages>> "%OUT%"
type "%TMPFILE%">> "%OUT%"
echo.>> "%OUT%"
type "%TMPFILE%"
echo.

set N=0
:download
set /a N+=1
curl.exe -s -f -H "%SITE%" -o "pagina-%N%.jpg" %AGENT%/pages/%N%
if not errorlevel 1 goto download
del /q "pagina-%N%.jpg" 2>nul
set /a N-=1
echo Facciate scaricate: %N%
echo Facciate scaricate: %N%>> "%OUT%"
curl.exe -s -X DELETE -H "%SITE%" %AGENT%/pages >nul

:log
echo --- agent.log>> "%OUT%"
type "%APPDATA%\FamilyManagerAgent\agent.log">> "%OUT%" 2>nul
del /q "%TMPFILE%" 2>nul
echo.
echo Fatto: il risultato e' in "%~dp0%OUT%". Va riportato cosi' com'e'.
pause
