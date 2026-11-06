# Unagikuma

<p align="center"><img src="docs/images/IMG_8128.jpeg" alt="Unagikuma" width="240"></p>

**Otra herramienta más para comprobar la disponibilidad: u(ptime) + nagi(os) + kuma(Uptime Kuma).**

**Nadar y guardar la ropa:** un archivo en una máquina funciona al momento; cuando quieras más, une otras máquinas y se convierte en un clúster. El mismo programa, nada que migrar.

Monitorización autoalojada y sencilla: entérate en cuanto tus webs o servidores caen, sin ruido.

[English](README.md) · [中文](README.zh.md) · [हिन्दी](README.hi.md) · **Español** · [العربية](README.ar.md) · [Français](README.fr.md) · [বাংলা](README.bn.md) · [Português](README.pt.md) · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · [Deutsch](README.de.md) · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

![La lista de monitores en el tono Papel](docs/images/monitors-paper.png)

## ¿Por qué Unagikuma?

- **Listo en unos 30 segundos.** Descarga un archivo, ejecútalo y abre el navegador.
- **Un solo archivo, nada más que instalar.** Funciona en Linux, macOS, Windows y FreeBSD.
- **Vigila lo importante.** Sitios web, servidores, puertos, ping, DNS, certificados SSL y caducidad de dominios.
- **Alertas donde ya estás.** Correo, Telegram, Slack, Discord, Feishu, DingTalk, WeCom, webhooks y más.
- **Páginas de estado** para mantener informados a tus usuarios.
- **Cambiarse es fácil.** Trae tus monitores desde Uptime Kuma o Nagios.
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

## Una máquina, dos máquinas, un clúster

- **Una máquina:** ejecuta el archivo, añade un monitor y listo. Sin base de datos ni agentes.
- **Dos máquinas:** pega un código de unión en **Ajustes › Nodos**. Cada máquina comprueba y alerta por su cuenta; si una cae, las alertas siguen llegando.
- **Un clúster:** añade tantos sitios como quieras. Cada alerta muestra lo que ve cada sitio, para distinguir «caído en todas partes» de «caído desde aquí».

![Un monitor caído con el resultado de cada sitio](docs/images/detail-sites.png)

## Sin avalanchas de alertas

- **¿Se cayó tu propia red?** Un mensaje sobre la red, no uno por monitor.
- **¿Se cayó un host?** Sus servicios no se avisan uno por uno.
- **Como mucho tres alertas por minuto y canal;** el resto llega en un solo resumen. Las recuperaciones salen siempre al instante.

## Cambiar desde Uptime Kuma

Detén Uptime Kuma y revisa primero (no se cambia nada):

```sh
./unagikuma import kuma --dry-run /path/to/uptime-kuma/data/kuma.db
```

Si todo se ve bien, importa:

```sh
./unagikuma import kuma /path/to/uptime-kuma/data/kuma.db
```

El informe muestra qué se importó, qué conviene revisar y qué no se pudo importar, con qué hacer en cada caso. También se guarda en `data/import-reports/`. Si lo ejecutas de nuevo, solo añade lo nuevo.

## Cambiar desde Nagios

Indica tu `nagios.cfg` y revisa primero (no se cambia nada):

```sh
./unagikuma import nagios --dry-run /etc/nagios4/nagios.cfg
```

Si todo se ve bien, importa:

```sh
./unagikuma import nagios /etc/nagios4/nagios.cfg
```

El informe muestra qué se importó, qué conviene revisar y qué no se pudo importar, con qué hacer en cada caso. También se guarda en `data/import-reports/`. Si lo ejecutas de nuevo, solo añade lo nuevo.

## ¿Olvidaste tu contraseña?

En la máquina donde se ejecuta Unagikuma:

```sh
./unagikuma reset-password admin
```

Escribe la nueva contraseña dos veces (no se muestra). Añade `--generate` para obtener una aleatoria.

## Más información

- [Manual de usuario (en inglés)](docs/manual.md) — todo lo demás, paso a paso
- [Registro de cambios](CHANGELOG.md)

## Licencia

Apache License 2.0, consulta [LICENSE](LICENSE).
