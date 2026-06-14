#Requires -Version 5.1
#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Install the sparkyctrl worker on Windows. The counterpart to deploy/install.sh.

.DESCRIPTION
    Downloads the prebuilt worker (or uses a local binary), installs it under Program Files,
    opens a Private-profile firewall rule for its port, and registers a Scheduled Task that
    runs it as SYSTEM at startup with auto-restart on crash. Re-run any time to update.
    Use -Uninstall to remove the task and firewall rule.

    WARNING: this installs a SYSTEM-level (root-equivalent) remote-code-execution daemon that
    listens on your network with no real authentication. Trusted LAN only. Read the README
    before you run this. There is no undo beyond -Uninstall and regret.

.EXAMPLE
    .\install.ps1 -Fence C:\share -Start

.EXAMPLE
    .\install.ps1 -Uninstall
#>
[CmdletBinding()]
param(
    [string]$Addr        = "0.0.0.0:7766",
    [string]$Fence       = "",
    [string]$Audit       = "C:\ProgramData\sparkyctrl\audit.log",
    [string]$Token       = "",
    [string]$Binary      = "",
    [string]$Version     = "latest",
    [string]$Repo        = "SimpleHonors/sparkyctrl",
    [string]$InstallDir  = "$Env:ProgramFiles\sparkyctrl",
    [string]$ServiceName = "sparkyctrl",
    [switch]$Start,
    [switch]$Uninstall
)

$ErrorActionPreference = "Stop"
function Info($m) { Write-Host "==> $m" }

$port = ($Addr -split ":")[-1]

if ($Uninstall) {
    if (Get-ScheduledTask -TaskName $ServiceName -ErrorAction SilentlyContinue) {
        Stop-ScheduledTask -TaskName $ServiceName -ErrorAction SilentlyContinue
        Unregister-ScheduledTask -TaskName $ServiceName -Confirm:$false
        Info "removed scheduled task '$ServiceName'"
    } else { Info "no scheduled task '$ServiceName' found" }
    Get-NetFirewallRule -DisplayName "sparkyctrl ($port)" -ErrorAction SilentlyContinue |
        Remove-NetFirewallRule -ErrorAction SilentlyContinue
    Info "left the binary + data in $InstallDir (delete manually if you want them gone)."
    return
}

# 1. Resolve the binary. An explicit -Binary uses a local file; otherwise download the
#    release. We deliberately do NOT auto-pick a binary from the current directory: a
#    planted .\sparkyctrl.exe would otherwise be installed and run as SYSTEM.
$dest = Join-Path $InstallDir "sparkyctrl.exe"
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

if ($Binary) {
    $src = (Resolve-Path -LiteralPath $Binary).Path
    Copy-Item -LiteralPath $src -Destination $dest -Force
    Info "installed binary: $src -> $dest"
} else {
    if ($Version -eq "latest") {
        $url = "https://github.com/$Repo/releases/latest/download/sparkyctrl-windows-amd64.exe"
    } else {
        $url = "https://github.com/$Repo/releases/download/$Version/sparkyctrl-windows-amd64.exe"
    }
    Info "downloading $url"
    Invoke-WebRequest -Uri $url -OutFile $dest -UseBasicParsing
}

# 2. Make sure the fence and audit-log locations exist.
if ($Fence) { New-Item -ItemType Directory -Force -Path $Fence | Out-Null }
New-Item -ItemType Directory -Force -Path (Split-Path $Audit) | Out-Null

# 3. Assemble the serve arguments.
$serveArgs = @("serve", "--addr", $Addr, "--audit", "`"$Audit`"")
if ($Fence) { $serveArgs += @("--fence", "`"$Fence`"") }
if ($Token) { $serveArgs += @("--token", $Token) }

# 4. Firewall: allow the port inbound on the Private profile only (never Public/internet).
Get-NetFirewallRule -DisplayName "sparkyctrl ($port)" -ErrorAction SilentlyContinue |
    Remove-NetFirewallRule -ErrorAction SilentlyContinue
New-NetFirewallRule -DisplayName "sparkyctrl ($port)" -Direction Inbound -Action Allow `
    -Protocol TCP -LocalPort $port -Profile Private -ErrorAction SilentlyContinue | Out-Null
Info "firewall: allowed inbound TCP $port on the Private profile"

# 5. Register a boot-start, auto-restarting Scheduled Task running as SYSTEM.
$action    = New-ScheduledTaskAction -Execute $dest -Argument ($serveArgs -join " ")
$trigger   = New-ScheduledTaskTrigger -AtStartup
$principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest
$settings  = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
                -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) `
                -ExecutionTimeLimit ([TimeSpan]::Zero)

Register-ScheduledTask -TaskName $ServiceName -Action $action -Trigger $trigger `
    -Principal $principal -Settings $settings -Force | Out-Null
Info "registered scheduled task '$ServiceName' (runs as SYSTEM at startup, auto-restarts)"

if ($Start) {
    Start-ScheduledTask -TaskName $ServiceName
    Start-Sleep -Seconds 1
    Info "started: $((Get-ScheduledTask -TaskName $ServiceName).State)"
}

Info "done."
Write-Host ""
Write-Host "sparkyctrl is installed as SYSTEM and listening on $Addr." -ForegroundColor Yellow
Write-Host "This is a root-equivalent remote shell. Trusted LAN only. Remove with: .\install.ps1 -Uninstall" -ForegroundColor Yellow
