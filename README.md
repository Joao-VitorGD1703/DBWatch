# DBWatch 👁️

**DBWatch** é uma ferramenta premium de monitoramento de bancos de dados em tempo real. Ela foi construída com um foco intenso em design e eficiência, fornecendo insights instantâneos sobre a saúde, queries lentas e sessões ativas da sua infraestrutura de dados.

Utilizando **Go** no backend para uma coleta de métricas de alta performance e **React (Carbon Design System)** no frontend, o DBX-Ray oferece um dashboard poderoso e fluído, capaz de identificar gargalos antes que eles derrubem sua aplicação.

## 🚀 Recursos Principais

- **Monitoramento Multi-Database:** Suporte nativo e simultâneo para PostgreSQL, MySQL e MongoDB.
- **Sessões Ativas (Tempo Real):** Acompanhe exatamente o que está rodando no banco *neste exato segundo* (Active Queries).
- **Métricas Chave:** Acompanhamento de Transações por Segundo (TPS), uso de disco, deadlocks, *locks waiting*, e *Cache Hit Ratio*.
- **Top Queries Lentas:** Histórico das consultas que mais consomem tempo de CPU da sua infraestrutura.
- **Gráficos Dinâmicos:** Integração via WebSocket para atualização ao vivo das barras e gráficos.

## 🛠️ Stack Tecnológico

- **Backend:** Go (Fiber, WebSocket, drivers nativos: `pgx`, `go-sql-driver/mysql`, `mongo-driver`)
- **Time-Series DB:** TimescaleDB (para armazenamento interno do histórico das métricas)
- **Frontend:** React, TypeScript, Vite, Recharts, e **IBM Carbon Design System**.
- **Infraestrutura:** Docker e Docker Compose.

---

## ⚙️ Como Baixar e Rodar (Passo a Passo)

### 1. Pré-requisitos
Antes de começar, certifique-se de ter instalado em sua máquina:
- [Docker](https://docs.docker.com/get-docker/)
- [Docker Compose](https://docs.docker.com/compose/install/)
- `make` (Em ambientes Windows sem o Make, os comandos Docker correspondentes podem ser executados manualmente).

### 2. Clone o Repositório
Baixe o código-fonte para a sua máquina e entre na pasta:
```bash
git clone <URL_DO_SEU_REPOSITORIO>
cd DBWatch
```

### 3. Inicie o Ambiente de Desenvolvimento
Toda a aplicação já está perfeitamente conteinerizada. Tudo o que você precisa fazer é executar um único comando para construir o Backend, subir o banco de dados interno (TimescaleDB), iniciar bancos de teste e carregar a interface (Frontend):

```bash
make dev
```
*(Se você não usa o `make`, pode rodar diretamente: `docker compose up --build`)*

### 4. Acesse o Dashboard
Com os containers rodando, abra seu navegador e acesse a interface web:
👉 **[http://localhost:5173](http://localhost:5173)**

---

## 🔌 Como Configurar seus Bancos de Dados

No canto superior direito do Dashboard, clique no botão azul **"Add Database"**.
Você poderá adicionar quantos bancos de dados quiser, de diferentes tecnologias. Para cada um, basta fornecer um nome identificador e a **Connection String (DSN)** adequada:

### Exemplos de Connection Strings aceitas:

- **PostgreSQL / Supabase / Neon:** 
  `postgres://usuario:senha@host:5432/nomedobanco`
- **MySQL / PlanetScale:** 
  `usuario:senha@tcp(host:3306)/nomedobanco`
- **MongoDB / MongoDB Atlas:** 
  `mongodb+srv://usuario:senha@cluster0.exemplo.mongodb.net/?retryWrites=true&w=majority`

Assim que você confirmar, o backend validará a conexão e as métricas começarão a piscar na sua tela instantaneamente!

---

*Desenvolvido com ❤️ para dominar o monitoramento de infraestruturas modernas.*
