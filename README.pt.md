# Unagikuma

<p align="center"><img src="docs/images/IMG_8128.jpeg" alt="Unagikuma" width="240"></p>

**Mais uma ferramenta de verificação de disponibilidade: u(ptime) + nagi(os) + kuma(Uptime Kuma).**

Monitoramento auto-hospedado e descomplicado: saiba na hora quando seus sites e servidores caem, sem barulho.

[English](README.md) · [中文](README.zh.md) · [हिन्दी](README.hi.md) · [Español](README.es.md) · [العربية](README.ar.md) · [Français](README.fr.md) · [বাংলা](README.bn.md) · **Português** · [Русский](README.ru.md) · [Bahasa Indonesia](README.id.md) · [اردو](README.ur.md) · [Deutsch](README.de.md) · [日本語](README.ja.md) · [Tiếng Việt](README.vi.md) · [한국어](README.ko.md)

![A lista de monitores no tom Papel](docs/images/monitors-paper.png)

## Por que o Unagikuma

- **Pronto em cerca de 30 segundos.** Baixe um arquivo, execute e abra o navegador.
- **Um único arquivo, nada mais para instalar.** Funciona em Linux, macOS, Windows e FreeBSD.
- **Verifica o que importa.** Sites, servidores, portas, ping, DNS, certificados SSL e vencimento de domínios.
- **Alertas onde você já está.** E-mail, Telegram, Slack, Discord, Feishu, DingTalk, WeCom, webhooks e muito mais.
- **Um problema, não uma enxurrada de alertas.** Se a sua própria rede cair, o Unagikuma percebe e não dispara um alerta para cada monitor.
- **Páginas de status** para manter seus usuários informados.
- **Cresce com você.** Adicione uma segunda máquina ou um cluster inteiro em um minuto quando precisar, com um código na página web.
- **Fácil de migrar.** Traga seus monitores do Uptime Kuma ou do Nagios.
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

## Uma máquina, duas máquinas, um cluster

> **O melhor dos dois mundos.** A maioria das ferramentas obriga você a escolher: uma pequena, fácil de começar mas presa a uma máquina, ou uma grande, que escala mas exige banco de dados, agentes e uma tarde de configuração. O Unagikuma é as duas coisas. Comece com um arquivo em uma máquina; quando quiser mais, junte outras máquinas a ela. O mesmo programa, a mesma interface, nada para migrar.

O Unagikuma é feito antes de tudo para uma única máquina. Tudo funciona nela: verificações, alertas, páginas de status, histórico. Mais máquinas só acrescentam mais pontos de vista.

### Uma máquina: pronto na hora

Execute o arquivo e adicione um monitor. Não há mais nada para configurar: nem banco de dados, nem agentes, nem segundo servidor. É assim que a maioria usa, e é suficiente por si só.

![A visão geral: onde quebra, desta máquina até cada destino](docs/images/overview.png)

### Duas máquinas: cada uma vigia por conta própria

Coloque um segundo servidor em outro lugar, por exemplo em outro escritório ou em outro provedor. No primeiro, abra **Ajustes › Nós › Código de adesão**; no segundo, **Ajustes › Nós › Entrar no cluster** e cole o código. O segundo servidor recebe na hora uma cópia da configuração e começa a verificar imediatamente.

- **Os dois servidores verificam e alertam de forma independente.** Se um cair, o outro continua funcionando e enviando alertas.
- **Sem adivinhação.** Quando dois servidores se perdem de vista, nenhum consegue saber qual lado ficou isolado, então nenhum declara o outro morto. Os dois continuam verificando e alertando por conta própria.
- **Cada mensagem informa o site de origem** (`[Tokyo] …`), para você sempre saber de onde veio um alerta.

### Um cluster: todos os sites, uma só imagem

Adicione quantos sites quiser do mesmo jeito. Cada site continua verificando e alertando sozinho, então perder qualquer um, inclusive o servidor principal, nunca interrompe o monitoramento. O que o cluster acrescenta é contexto: cada monitor e cada alerta mostram o que os outros sites estão vendo agora.

![Um monitor com falha e o resultado de cada site](docs/images/detail-sites.png)

- **"Fora do ar em todo lugar" ou "fora do ar daqui"?** As observações de cada site mostram num relance se um serviço caiu de verdade ou se só um caminho de rede quebrou. Resultados desatualizados são marcados como tal.
- **Um site que fica em silêncio é avisado uma única vez,** depois de um minuto (ajustável), com aviso de recuperação quando volta. Reiniciar o servidor principal não dispara uma onda de "site inacessível".
- **As alterações chegam a todos os sites em segundos,** e cada site pode manter monitores próprios, como o roteador daquele escritório.

![A página de nós: cada site, seu tempo de ida e volta e sua configuração](docs/images/nodes.png)

## Enxurrada de alertas: um problema, uma mensagem

Uma ferramenta que chama você cem vezes por um cabo rompido acaba silenciada e, aí, inútil. O Unagikuma trata uma enxurrada de alertas como um defeito a evitar:

- **Primeiro a sua própria rede.** Quando esta máquina perde a própria conexão com a internet, o Unagikuma percebe que todos os destinos falharam juntos e envia uma mensagem sobre a rede em vez de uma por monitor. Quando a rede volta, os monitores retidos são verificados de novo aos poucos antes de qualquer aviso.
- **Pais antes dos filhos.** Quando um host cai, os serviços dele não são avisados um a um.
- **No máximo três alertas por minuto por canal.** O excedente vira um único resumo no fim do minuto, o mais grave primeiro. Problemas novos têm cota própria, então lembretes nunca os atrasam, mas cem serviços caindo juntos ainda chegam como um resumo, não como cem mensagens.
- **Boas notícias nunca ficam retidas.** Recuperações e escalonamentos sempre saem na hora: você nunca espera pela mensagem que diz que pode parar.
- **Oscilações são resumidas.** Um serviço que sobe e desce é informado como um único incidente com contagem.
- **Trabalho planejado fica em silêncio.** Janelas de manutenção silenciam aquilo em que você avisou que ia mexer.

Todos os limites podem ser ajustados em **Ajustes › Parâmetros**.

## Vindo do Uptime Kuma

Pare o Uptime Kuma e confira antes (nada é alterado):

```sh
./unagikuma import kuma --dry-run /path/to/uptime-kuma/data/kuma.db
```

Se estiver tudo certo, importe:

```sh
./unagikuma import kuma /path/to/uptime-kuma/data/kuma.db
```

O relatório mostra o que foi importado, o que precisa de atenção e o que não pôde ser importado, com o que fazer. Ele também é salvo em `data/import-reports/`. Rodar de novo só adiciona o que for novo.

## Vindo do Nagios

Aponte para o seu `nagios.cfg` e confira antes (nada é alterado):

```sh
./unagikuma import nagios --dry-run /etc/nagios4/nagios.cfg
```

Se estiver tudo certo, importe:

```sh
./unagikuma import nagios /etc/nagios4/nagios.cfg
```

O relatório mostra o que foi importado, o que precisa de atenção e o que não pôde ser importado, com o que fazer. Ele também é salvo em `data/import-reports/`. Rodar de novo só adiciona o que for novo.

## Esqueceu a senha?

Na máquina onde o Unagikuma roda:

```sh
./unagikuma reset-password admin
```

Digite a nova senha duas vezes (ela não aparece). Adicione `--generate` para receber uma aleatória.

## Saiba mais

- [Manual do usuário (em inglês)](docs/manual.md) — todo o resto, passo a passo
- [Histórico de mudanças](CHANGELOG.md)

## Licença

Apache License 2.0, veja [LICENSE](LICENSE).
