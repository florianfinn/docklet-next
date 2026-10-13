/* Starting draft for a new stack; unknown keys show that the editor keeps them. */
export const NEW_STACK_DRAFT = `# Neuer Entwurf – unbekannte Felder bleiben beim Umschalten erhalten
name: whoami
x-docklet:
  notiz: Dieses Feld kennt das Formular nicht und bleibt trotzdem erhalten
services:
  web:
    image: traefik/whoami:v1.11
    restart: unless-stopped
    ports:
      - "8081:80"
    environment:
      TZ: Europe/Berlin
    labels:
      - traefik.http.routers.whoami.rule=Host(\`whoami.example.com\`)
`;
