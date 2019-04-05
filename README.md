### Introduction / Giris

This operator is created to manage some Kubernetes pods that retrieve the election data for Turkey in 2019.
It creates and manages bunch of pods of this image: <https://github.com/aliok/secim-batch-container>
Then collects their results and saves it locally.

Collected and saved result can be analyzed using a script like this one: <https://gist.github.com/aliok/5b739b6273ec37fb58eb6e90012e5e17>

I implemented this operator to practice Kubernetes operators. Kubernetes Jobs would be a better fit for this kind of purpose in production.
I intentionally didn't use the Operator Framework as I wanted to compare things written using that one and without that one.

-----

Bu operator, Kubernetes uzerinde 2019 yerel secimlerinin sonuclarini toplayan podlari yonetmek icin olusturuldu. 
Su imaj <https://github.com/aliok/secim-batch-container> ile pod olusturup bunlari yonetir. Podlarin topladigi verileri
alir ve kaydeder.

Kaydedilen sonuclari suradaki gibi bir script ile analiz edebilirsiniz: <https://gist.github.com/aliok/5b739b6273ec37fb58eb6e90012e5e17>

Bu operatoru Kubernetes operatorleri ile pratik yapmak icin olusturdum. Bir production ortaminda Kubernetes Jobs konseptini kullanmak daha makul. 


### Gelistirilebilecek seyler / Room for improvement

1. Externalizing stuff hardcoded in the source tree (job id, batch size, image name, polling period etc.)
2. Configure in-cluster or out-of-cluster config based on some env var
3. Better error handling
4. Annotating the pods with a special annotation that includes the job id. Currently pods are created with no separation from other pods.
   This is because I was simply lazy and wanted to have something running.

-----

1. Kod icine gomulmus degerlerin haricten verilmesi (job id, batch size, image name, polling period vs.)
2. Cluster ici ve disi config'in bir env var'a gore secilmesi
3. Daha iyi hata yonetimi
4. Pod'lara job id annotation'u verme. Su anda podlar dumduz olusturuluyor ve diger podlardan ayirdedilmiyor. Hizlica yazdigim icin bu boyle.

### Setup / Kurulum

```
make setup
```

### Running locally / Lokalde calistirma

```
# project name is hardcoded as "myproject" in the code. Feel free to send a PR to receive it from env vars.
kubectl create namespace myproject

make build_linux
./main
```

### Running in the cluster / Cluster icinde calistirma

The operator code is currently using a hardcoded out-of-cluster config. That means, if you want to run it in the cluster,
you need to modify the code LOL :)

This is not that much of a work, but it is not in my point of experimenting with Kubernetes API. I don't care! 