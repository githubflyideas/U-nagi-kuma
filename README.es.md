# Unagikuma

<p align="center"><img src="docs/images/logo.png" alt="Unagikuma" width="240"></p>

**Otra herramienta más para comprobar la disponibilidad: u(ptime) + nagi(os) + kuma(Uptime Kuma).**

Monitorización autoalojada y sencilla: entérate en cuanto tus webs o servidores caen, sin ruido.

[English](README.md) · [中文](README.zh.md) · [हिन्दी](README.hi.md) · **Español** · [العربية](README.ar.md) · [Français](README.fr.md) · [বাংলা](README.bn.md) · [Português](README.pt.md) · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · [Deutsch](README.de.md) · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

![Un breve recorrido por la interfaz web de Unagikuma](docs/images/tour.gif)

## ¿Por qué Unagikuma?

- **Listo en unos 30 segundos.** Descarga un archivo, ejecútalo y abre el navegador.
- **Un solo archivo, nada más que instalar.** Funciona en Linux, macOS, Windows y FreeBSD.
- **Vigila lo importante.** Sitios web, servidores, puertos, ping, DNS, certificados SSL y caducidad de dominios.
- **Alertas donde ya estás.** Correo, Telegram, Slack, Discord, Feishu, DingTalk, WeCom, webhooks y más.
- **Un problema, no una avalancha de alertas.** Si se cae tu propia red, Unagikuma se da cuenta y no te avisa por cada monitor.
- **Páginas de estado** para mantener informados a tus usuarios.
- **Crece contigo.** Comprueba desde varias ubicaciones cuando lo necesites (versión preliminar).
- **Cambiarse es fácil.** Trae tus monitores desde Uptime Kuma o Nagios (versión preliminar).
- **Habla tu idioma.** La interfaz está disponible en 15 idiomas.

## Primeros pasos

**1.** Descarga el archivo para tu sistema desde [Releases](https://github.com/githubflyideas/unagikuma/releases/latest) y descomprímelo.

**2.** Ejecútalo:

```sh
./unagikuma
```

En Windows, ejecuta `unagikuma.exe`.

**3.** Abre http://localhost:8077 en el navegador y crea tu cuenta de administrador.

**4.** Haz clic en **Añadir monitor**, escribe la dirección de una web y listo.

> ¿Lo abres desde otro ordenador? Entra en `http://<dirección-del-servidor>:8077` e introduce el **Token de configuración** que mostró el programa al arrancar por primera vez.

## ¿Prefieres Docker?

```sh
docker run -d --name unagikuma -p 8077:8077 -v unagikuma-data:/data ghcr.io/githubflyideas/unagikuma:latest
```

Después abre http://localhost:8077. Encontrarás el **Token de configuración** con `docker logs unagikuma`.

## Más información

- [Manual de usuario (en inglés)](docs/manual.md) — todo lo demás, paso a paso
- [Registro de cambios](CHANGELOG.md)

## Licencia

Apache License 2.0, consulta [LICENSE](LICENSE).
