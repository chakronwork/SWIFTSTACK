# Build portable ZIP package for webserver stack
param (
    [string]$Version = "0.1.0"
)

$ErrorActionPreference = "Stop"
$RootDir = Split-Path -Parent $PSScriptRoot
$DistDir = Join-Path $RootDir "dist"
$StagingDir = Join-Path $DistDir "staging"
$ZipName = "webserver-v$Version-windows-x64-portable.zip"
$ZipPath = Join-Path $DistDir $ZipName

Write-Host "Building webserver v$Version portable package..." -ForegroundColor Cyan

# 1. Clean staging directory
if (Test-Path $StagingDir) {
    Remove-Item -Recurse -Force $StagingDir
}
New-Item -ItemType Directory -Path $StagingDir | Out-Null
New-Item -ItemType Directory -Path $DistDir -Force | Out-Null

# 2. Build Go binary
Write-Host "Compiling webserver.exe..." -ForegroundColor Yellow
Set-Location $RootDir
& "C:\Program Files\Go\bin\go.exe" build -ldflags "-s -w" -o (Join-Path $StagingDir "webserver.exe") ./cmd/webserver

# 3. Copy required base files & templates
Write-Host "Copying configuration, templates, and licenses..." -ForegroundColor Yellow
Copy-Item (Join-Path $RootDir "stack.toml") $StagingDir
Copy-Item (Join-Path $RootDir "versions.toml") $StagingDir
Copy-Item -Recurse (Join-Path $RootDir "templates") (Join-Path $StagingDir "templates")
Copy-Item -Recurse (Join-Path $RootDir "licenses") (Join-Path $StagingDir "licenses")

# 4. Create empty directory placeholders
$emptyDirs = @("bin", "conf\vhosts", "conf\ssl", "www", "data\mysql", "logs", "tmp", "sessions", "backups")
foreach ($dir in $emptyDirs) {
    New-Item -ItemType Directory -Path (Join-Path $StagingDir $dir) -Force | Out-Null
}

# 5. Create default index.php in www/
$welcomeContent = @'
<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Welcome to webserver</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #0f172a; color: #f8fafc; display: flex; justify-content: center; align-items: center; min-height: 100vh; margin: 0; }
        .card { background: #1e293b; padding: 2rem 3rem; border-radius: 12px; box-shadow: 0 10px 25px rgba(0,0,0,0.5); text-align: center; max-width: 500px; }
        h1 { color: #38bdf8; margin-top: 0; }
        p { color: #94a3b8; font-size: 1.1rem; line-height: 1.6; }
        .badge { display: inline-block; padding: 0.25rem 0.75rem; background: #0284c7; color: white; border-radius: 9999px; font-weight: bold; margin-bottom: 1rem; }
        a { color: #38bdf8; text-decoration: none; font-weight: 500; }
        a:hover { text-decoration: underline; }
    </style>
</head>
<body>
    <div class="card">
        <div class="badge">PHP <?php echo PHP_VERSION; ?></div>
        <h1>webserver is running!</h1>
        <p>Portable, lightweight & secure local PHP stack for Windows.</p>
        <p><a href="phpinfo.php">View phpinfo()</a></p>
    </div>
</body>
</html>
'@
Set-Content -Path (Join-Path $StagingDir "www\index.php") -Value $welcomeContent
Set-Content -Path (Join-Path $StagingDir "www\phpinfo.php") -Value "<?php phpinfo(); ?>"

# 6. Compress into ZIP archive
if (Test-Path $ZipPath) {
    Remove-Item -Force $ZipPath
}
Write-Host "Creating portable ZIP archive: $ZipPath..." -ForegroundColor Yellow
Compress-Archive -Path "$StagingDir\*" -DestinationPath $ZipPath

# 7. Clean up staging
Remove-Item -Recurse -Force $StagingDir

$fileSize = (Get-Item $ZipPath).Length / 1MB
Write-Host "Successfully generated portable package: $ZipName ($([math]::Round($fileSize, 2)) MB)" -ForegroundColor Green
