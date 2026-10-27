# Installs OpenSSH Server while building the Windows base image.
# Run from autounattend.xml (FirstLogonCommands) as F:\setup-openssh.ps1.
# It is a separate file because a CommandLine in an answer file is limited to 1024 characters.
# build-windows-qcow2.sh puts it on the assets ISO.
# Keep this file ASCII-only: Windows PowerShell 5.1 reads BOM-less files as ANSI.
#
# - Install OpenSSH Server (needs an Internet connection).
# - Set sshd to start automatically.
# - Set PowerShell as the default shell.
#
# sshd is NOT started here. Starting it generates host keys, and every VM created from the
# sysprep'ed image would share them. marmot places the public keys, opens TCP 22 and starts
# sshd at server creation via cloudbase-init (runcmd).
# Always exit 0 so that the following steps (sysprep) are not blocked.
$ErrorActionPreference = 'Continue'

Add-WindowsCapability -Online -Name 'OpenSSH.Server~~~~0.0.1.0' | Out-Null
Set-Service -Name sshd -StartupType Automatic

$key = 'HKLM:\SOFTWARE\OpenSSH'
New-Item -Path $key -Force | Out-Null
New-ItemProperty -Path $key -Name DefaultShell -PropertyType String -Force `
    -Value 'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe' | Out-Null

exit 0
