#!/bin/bash

echo "=========================================="
echo "  GoFileBeam - Git Setup"
echo "=========================================="
echo ""

# Prompt for user information
read -p "Enter your GitHub username (Suleman-Elahi): " GIT_USER
GIT_USER=${GIT_USER:-Suleman-Elahi}

read -p "Enter your GitHub email: " GIT_EMAIL

if [ -z "$GIT_EMAIL" ]; then
    echo "Error: Email is required"
    exit 1
fi

echo ""
echo "Configuring git for this repository only..."
echo ""

# Set local git config (only for this repo)
git config --local user.name "$GIT_USER"
git config --local user.email "$GIT_EMAIL"

# Set default branch to main
git branch -M main

# Add remote
git remote add origin https://github.com/Suleman-Elahi/GoFileBeam.git

echo "✓ Git configured locally for this repository"
echo ""
echo "Configuration:"
echo "  User: $GIT_USER"
echo "  Email: $GIT_EMAIL"
echo "  Remote: https://github.com/Suleman-Elahi/GoFileBeam.git"
echo "  Branch: main"
echo ""
echo "Verifying local config:"
git config --local --list | grep user
echo ""
echo "=========================================="
echo "Next steps:"
echo "=========================================="
echo ""
echo "1. Add files to git:"
echo "   git add ."
echo ""
echo "2. Create initial commit:"
echo "   git commit -m 'Initial commit: GoFileBeam secure file sharing'"
echo ""
echo "3. Push to GitHub:"
echo "   git push -u origin main"
echo ""
echo "Note: You may be prompted for GitHub credentials."
echo "Consider using a Personal Access Token instead of password."
echo ""
