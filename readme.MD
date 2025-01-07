ShounicBestClips
===

Setup
---
* Install [Golang](https://go.dev/doc/install)
* `git clone` the project
* `go mod tidy` to install get dependencies
* `go run .` to run the project on port `8081`

### Docker
* Install [Docker](https://www.docker.com)
* `docker build . -t shounic-best-clips` to build the docker image
* `docker run --rm -p 8081:8081 -v shounic-best-clips-data:/app/data shounic-best-clips` to run the container
* `docker volume rm shounic-best-clips-data` to delete the database volume

Routes
---
* `/` Main page
* `/vote/next` Get the next vote for the user returns the next vote.
* `/vote/submit?choice=` Submit a vote, returns the next vote.
* `/vote/deadline` Get the voting deadline as a unix time string

`/vote/next` returns the following json for the next vote:
```
{
    "a": string,
    "b": string
}
```

If the route returns `204`, then the user has completed all their votes.
If the route returns `420`, then a predetermined deadline has passed and voting is no longer allowed.
