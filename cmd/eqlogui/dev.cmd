@echo off
setlocal

REM Start the frontend dev server in a separate window
start "eqlogui-frontend" cmd /c "cd frontend && npm run dev"

REM Give Vite a moment to start listening
timeout /t 2 /nobreak >nul

REM Start the Wails app using the external dev server
wails dev -s -frontenddevserverurl http://127.0.0.1:5173

endlocal
