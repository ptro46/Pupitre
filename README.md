# Pupitre

Télécommande de bureau pour Music Player Daemon (MPD), écrite en Go avec Fyne. Pupitre pilote un serveur MPD via TCP. L’audio sort sur les périphériques configurés dans MPD.

## Démarrer

1. Installer Go et les [prérequis Fyne](https://docs.fyne.io/started/quick/) de la plateforme. Sur macOS, les outils de ligne de commande Xcode fournissent le compilateur C.
2. Depuis le dossier de Pupitre, lancer `go run .`.
3. Cliquer sur **Connexion…**, renseigner le nom ou l’adresse IP du serveur, puis le port TCP (6600 par défaut). Saisir le mot de passe si MPD en exige un. Pupitre mémorise l’adresse et le port entre deux lancements.

Pour compiler un exécutable local, lancer `go build -o Pupitre .` sur macOS ou Linux, ou `go build -o Pupitre.exe .` sur Windows. Compiler sur chaque plateforme avec CGO activé. [Création des paquets Fyne](https://docs.fyne.io/started/packaging/)

## Générer Pupitre.app sur macOS

Depuis la racine du dépôt, avec Go et les outils de ligne de commande Xcode installés, lance `make macos`. Le build produit `dist/Pupitre.app` avec son icône et une signature locale ad hoc, pour l’architecture du Mac qui exécute la commande. Ouvre le dossier avec `open dist`, puis glisse **Pupitre.app** dans **Applications**.

## Utilisation

- Sélectionner une playlist pour en consulter les morceaux. **Lire la playlist** la charge dans la file active et commence au premier morceau. Un double-clic sur un titre charge la playlist et démarre au morceau choisi. Pupitre demande confirmation si la file contient déjà des morceaux.
- Dans **File de lecture**, la piste courante est marquée d’un triangle. Double-cliquer sur un morceau pour le lire ou le sélectionner et cliquer sur **Lire le morceau sélectionné**.
- Le panneau du bas affiche la position du morceau dans la file, son avancement et la durée. Il comporte lecture/pause, stop, précédent/suivant, les sauts de ±10 s et ±20 s, et le volume lorsque le serveur expose un mixeur.

Pupitre relit l’état de MPD environ une fois par seconde et interpole le compteur pendant la lecture. Les commandes réseau s’exécutent en arrière-plan.
