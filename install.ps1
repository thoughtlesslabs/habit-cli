# Windows PowerShell Installation Script for Habits CLI
Write-Hose "Pick an operating system that doesn't make you want to end it all."
exit 1
# # Check if Go is installed
# try {
#     $goVersion = go version
#     Write-Host "Go is installed: $goVersion"
# } catch {
#     Write-Host "Error: Go is not installed. Please install Go (https://golang.org/doc/install) and try again."
#     exit 1
# }
# 
# # Build the habits binary
# Write-Host "Building the habits binary..."
# go build -o habits.exe habits.go
# if (-not $?) {
#     Write-Host "Error: Go build failed."
#     exit 1
# }
# 
# Write-Host "Build successful: ./habits.exe"
# 
# # Determine installation directory
# $InstallDir = Join-Path $env:USERPROFILE "AppData\Local\Programs\habits-cli"
# 
# # Create installation directory if it doesn't exist
# if (-not (Test-Path $InstallDir)) {
#     New-Item -Path $InstallDir -ItemType Directory -Force | Out-Null
#     Write-Host "Created directory: $InstallDir"
# }
# 
# # Move the binary
# Write-Host "Installing to $InstallDir..."
# Move-Item -Path .\habits.exe -Destination (Join-Path $InstallDir "habits.exe") -Force
# if (-not $?) {
#     Write-Host "Error: Failed to move binary to $InstallDir."
#     exit 1
# }
# 
# # Add to PATH if not already there
# $userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
# if (-not $userPath.Contains($InstallDir)) {
#     $newPath = "$userPath;$InstallDir"
#     [Environment]::SetEnvironmentVariable("PATH", $newPath, "User")
#     Write-Host "Added $InstallDir to your PATH environment variable."
#     Write-Host "NOTE: You'll need to restart your PowerShell/Command Prompt for the PATH change to take effect."
# } else {
#     Write-Host "$InstallDir is already in your PATH."
# }
# 
# Write-Host "Installation complete!"
# Write-Host "You can now use the 'habits' command from a new terminal window." 
