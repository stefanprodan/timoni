# Installation Guide

Timoni is available as a binary executable for Linux, macOS and Windows.
The AMD64 and ARM64 binaries can be downloaded from GitHub [releases](https://github.com/stefanprodan/timoni/releases).

=== "Install with brew"

    Install the latest release on macOS or Linux with:
    
    ```shell
    brew install stefanprodan/tap/timoni
    ```

    Note that the Homebrew formula will setup shell autocompletion for Bash, Fish and Zsh.

=== "arkade"

    Install the latest release on Windows, macOS or Linux with:
    
    ```shell
    arkade get timoni
    ```

    Note that the [Arkade](https://github.com/alexellis/arkade) version must be 0.9.11 or newer.

=== "nix"

    Install the latest release with [nix-env](https://nixos.org/manual/nix/unstable/command-ref/nix-env.html):
    
    ```shell
    nix-env -i timoni
    ```

    Note that the Nix package will setup shell autocompletion for Bash, Fish and Zsh.

=== "yay"

    Install the latest release with [yay](https://github.com/Jguer/yay) (or another [AUR helper](https://wiki.archlinux.org/title/AUR_helpers)) for Arch Linux:
    
    ```shell
    yay -S timoni
    ```

    If you prefer to use the upstream binaries:

    ```shell
    yay -S timoni-bin
    ```

=== "zypper"

    Install the latest release with [zypper](https://github.com/openSUSE/zypper) for openSUSE:
    
    ```shell
    zypper install timoni
    ```

    To setup shell autocompletion:

    ```shell
    zypper install timoni-bash-completion
    zypper install timoni-fish-completion
    zypper install timoni-zsh-completion
    ```

=== "from source"

    Using Go >= 1.21:
    
    ```shell
    go install github.com/stefanprodan/timoni/cmd/timoni@latest
    ```

## Shell autocompletion

Configure your shell to load timoni completions:

=== "Bash"

    To load completion run:
    
    ```shell
    . <(timoni completion bash)
    ```

    To configure your bash shell to load completions for each session add to your bashrc:

    ```shell
    # ~/.bashrc or ~/.bash_profile
    command -v timoni >/dev/null && . <(timoni completion bash)
    ```

    If you have an alias for timoni, you can extend shell completion to work with that alias:

    ```shell
    # ~/.bashrc or ~/.bash_profile
    alias tm=timoni
    complete -F __start_timoni tm
    ```

=== "Fish"

    To configure your fish shell to [load completions](http://fishshell.com/docs/current/index.html#completion-own)
    for each session write this script to your completions dir:
    
    ```shell
    timoni completion fish > ~/.config/fish/completions/timoni.fish
    ```

=== "Powershell"

    To load completion run:

    ```shell
    . <(timoni completion powershell)
    ```

    To configure your powershell shell to load completions for each session add to your powershell profile:
    
    Windows:

    ```shell
    cd "$env:USERPROFILE\Documents\WindowsPowerShell\Modules"
    timoni completion >> timoni-completion.ps1
    ```
    Linux:

    ```shell
    cd "${XDG_CONFIG_HOME:-"$HOME/.config/"}/powershell/modules"
    timoni completion >> timoni-completions.ps1
    ```

=== "Zsh"

    To load completion run:
    
    ```shell
    . <(timoni completion zsh) && compdef _timoni timoni
    ```

    To configure your zsh shell to load completions for each session add to your zshrc:
    
    ```shell
    # ~/.zshrc or ~/.profile
    command -v timoni >/dev/null && . <(timoni completion zsh) && compdef _timoni timoni
    ```

    or write a cached file in one of the completion directories in your ${fpath}:
    
    ```shell
    echo "${fpath// /\n}" | grep -i completion
    timoni completion zsh > _timoni
    
    mv _timoni ~/.oh-my-zsh/completions  # oh-my-zsh
    mv _timoni ~/.zprezto/modules/completion/external/src/  # zprezto
    ```

## SLSA Provenance & SBOMs

The build, release and provenance portions of Timoni's supply chain meet the
[SLSA Build Level 3](https://slsa.dev/spec/v1.0/levels) requirements.

The release artifacts are produced on GitHub-hosted runners using
[GoReleaser](https://goreleaser.com) and the provenance generation is handled by the official
[SLSA GitHub Generator](https://github.com/slsa-framework/slsa-github-generator).

To verify a release artifact such as the Timoni binary tarball,
you can use the [slsa-verifier](https://github.com/slsa-framework/slsa-verifier) tool:

```shell
TIMONI_VER=0.10.0 && \
gh release download v${TIMONI_VER} -R=stefanprodan/timoni -p="*" && \
slsa-verifier verify-artifact \
--provenance-path timoni_${TIMONI_VER}_provenance.intoto.jsonl \
--source-uri github.com/stefanprodan/timoni  \
--source-tag v${TIMONI_VER} \
timoni_${TIMONI_VER}_darwin_arm64.tar.gz
```

Each release comes with a Software Bill of Materials (SBOM) in [SPDX](https://spdx.dev) format.
The SBOMs are generated on GitHub-hosted runners using
[GoReleaser](https://goreleaser.com) and [Syft](https://github.com/anchore/syft).

To scan a release for vulnerabilities, you can use [Grype](https://github.com/anchore/grype):

```shell
TIMONI_VER=0.10.0 && \
gh release download v${TIMONI_VER} -R=stefanprodan/timoni -p="*sbom.spdx.json" && \
grype sbom:./timoni_${TIMONI_VER}_sbom.spdx.json
```
