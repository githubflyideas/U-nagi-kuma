# Unagikuma

<p align="center"><img src="docs/images/logo.png" alt="Unagikuma" width="240"></p>

**Mais uma ferramenta de verificação de disponibilidade: u(ptime) + nagi(os) + kuma(Uptime Kuma).**

Monitoramento auto-hospedado e descomplicado: saiba na hora quando seus sites e servidores caem, sem barulho.

[English](README.md) · [中文](README.zh.md) · [हिन्दी](README.hi.md) · [Español](README.es.md) · [العربية](README.ar.md) · [Français](README.fr.md) · [বাংলা](README.bn.md) · **Português** · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · [Deutsch](README.de.md) · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

![Um tour rápido pela interface web do Unagikuma](docs/images/tour.gif)

## Por que o Unagikuma

- **Pronto em cerca de 30 segundos.** Baixe um arquivo, execute e abra o navegador.
- **Um único arquivo, nada mais para instalar.** Funciona em Linux, macOS, Windows e FreeBSD.
- **Verifica o que importa.** Sites, servidores, portas, ping, DNS, certificados SSL e vencimento de domínios.
- **Alertas onde você já está.** E-mail, Telegram, Slack, Discord, Feishu, DingTalk, WeCom, webhooks e muito mais.
- **Um problema, não uma enxurrada de alertas.** Se a sua própria rede cair, o Unagikuma percebe e não dispara um alerta para cada monitor.
- **Páginas de status** para manter seus usuários informados.
- **Cresce com você.** Verifique a partir de vários locais quando precisar (prévia).
- **Fácil de migrar.** Traga seus monitores do Uptime Kuma ou do Nagios (prévia).
- **Fala a sua língua.** A interface está disponível em 15 idiomas.

## Primeiros passos

**1.** Baixe o arquivo para o seu sistema em [Releases](https://github.com/githubflyideas/unagikuma/releases/latest) e descompacte.

**2.** Execute:

```sh
./unagikuma
```

No Windows, execute `unagikuma.exe`.

**3.** Abra http://localhost:8077 no navegador e crie sua conta de administrador.

**4.** Clique em **Adicionar monitor**, digite o endereço de um site e pronto.

> Vai abrir de outro computador? Acesse `http://<endereço-do-servidor>:8077` e informe o **Token de instalação** que o programa mostrou ao iniciar pela primeira vez.

## Prefere Docker?

```sh
docker run -d --name unagikuma -p 8077:8077 -v unagikuma-data:/data ghcr.io/githubflyideas/unagikuma:latest
```

Depois abra http://localhost:8077. O **Token de instalação** aparece com `docker logs unagikuma`.

## Saiba mais

- [Manual do usuário (em inglês)](docs/manual.md) — todo o resto, passo a passo
- [Histórico de mudanças](CHANGELOG.md)

## Licença

Apache License 2.0, veja [LICENSE](LICENSE).
