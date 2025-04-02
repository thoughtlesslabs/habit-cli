#!/bin/bash

set -e

# Check if Go is installed
if ! command -v go &> /dev/null
then
    echo "Error: Go is not installed. Please install Go (https://golang.org/doc/install) and try again."
    exit 1
fi

echo "Building the habits binary..."
go build -o habits habits.go
if [ $? -ne 0 ]; then
    echo "Error: Go build failed."
    exit 1
fi

echo "Build successful: ./habits"

# Determine installation directory
INSTALL_DIR="$HOME/.local/bin"

# Create installation directory if it doesn't exist
mkdir -p "$INSTALL_DIR"

echo "Attempting to install to $INSTALL_DIR..."
# Move the binary
mv ./habits "$INSTALL_DIR/habits"
if [ $? -ne 0 ]; then
    echo "Error: Failed to move binary to $INSTALL_DIR."
    echo "You might need to run this script with sudo, or manually move the './habits' binary."
    # Specify the install directory ($HOME/.local/bin) in the error message.
    exit 1
fi

chmod +x "$INSTALL_DIR/habits"

echo "Successfully installed habits to $INSTALL_DIR/habits"

# Check if the directory is in PATH
case ":$PATH:" in
    *:"$INSTALL_DIR":*) 
        echo "$INSTALL_DIR is already in your PATH."
        ;;
    *) 
        SHELL_TYPE=$(basename "$SHELL")
        CONFIG_FILE=""
        CONFIG_COMMAND=""
        
        if [ "$SHELL_TYPE" = "fish" ]; then
            # Fish shell configuration
            CONFIG_FILE="$HOME/.config/fish/config.fish"
            CONFIG_COMMAND="fish_add_path $INSTALL_DIR"
            
            # Create the fish config directory if it doesn't exist
            mkdir -p "$(dirname "$CONFIG_FILE")"
            
            # Check if file exists
            if [ ! -f "$CONFIG_FILE" ]; then
                touch "$CONFIG_FILE"
            fi
            
            # Check if the path is already in config.fish
            if ! grep -q "fish_add_path $INSTALL_DIR" "$CONFIG_FILE"; then
                echo "$CONFIG_COMMAND" >> "$CONFIG_FILE"
                echo "Added $INSTALL_DIR to your fish shell PATH in $CONFIG_FILE"
            fi
            
            echo ""
            echo "-----------------------------------------------------------------"
            echo " IMPORTANT: Reload your fish shell configuration"
            echo "-----------------------------------------------------------------"
            echo "To use 'habits' command immediately, run:"
            echo ""
            echo "  source $CONFIG_FILE"
            echo ""
            echo "Or simply restart your terminal."
            
        else
            # Bash/Zsh configuration
            if [ "$SHELL_TYPE" = "bash" ]; then
                CONFIG_FILE="$HOME/.bashrc"
            elif [ "$SHELL_TYPE" = "zsh" ]; then
                CONFIG_FILE="$HOME/.zshrc"
            else
                CONFIG_FILE="$HOME/.profile"
            fi
            
            CONFIG_COMMAND="export PATH=\"$INSTALL_DIR:\$PATH\""
            
            # To keep in line with the fish config notice, you should also specify `source ~/.bashrc` or `exec zsh`. 
            echo ""
            echo "-----------------------------------------------------------------"
            echo " IMPORTANT: Add $INSTALL_DIR to your PATH"
            echo "-----------------------------------------------------------------"
            echo "To run 'habits' from anywhere, you need to add $INSTALL_DIR to your PATH environment variable."
            echo "You can usually do this by adding the following line to your shell configuration file (e.g., ~/.bashrc, ~/.zshrc, ~/.profile):"
            echo ""
            echo "  $CONFIG_COMMAND"
            echo ""
            echo "After adding the line, restart your terminal or run 'source $CONFIG_FILE'."
        fi
        ;;
esac

echo "Installation complete." 
