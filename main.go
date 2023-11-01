package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"syscall"
	"time"

	pb "containerd-grpc/pb"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/api/services/tasks/v1"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

var (
	port = flag.Int("port", 50051, "Server port")
)

type ImageServer struct {
	pb.UnimplementedImageServiceServer
	client *containerd.Client
}

type NamespaceServer struct {
	pb.UnimplementedNamespaceServiceServer
	client *containerd.Client
}

type TaskServer struct {
	pb.UnimplementedTaskServiceServer
	client *containerd.Client
}

type ContainerServer struct {
	pb.UnimplementedContainerServiceServer
	client *containerd.Client
}

func (s *ImageServer) ListImages(ctx context.Context, in *pb.ListImagesRequest) (*pb.ListImagesResponse, error) {
	ns := namespaces.WithNamespace(context.Background(), in.GetNamespace())

	images, err := s.client.ListImages(ns)
	if err != nil {
		log.Printf("Failed to list images: %v", err)
		return nil, status.Error(codes.Aborted, err.Error())
	}

	pb_images := []*pb.Image{}

	for _, image := range images {
		size, err := image.Size(ns)
		if err != nil {
			log.Printf("Failed to read size of image: %v", err)
		}

		pb_images = append(pb_images[:], &pb.Image{
			Name:   image.Name(),
			Labels: image.Labels(),
			Size:   size,
		})
	}

	return &pb.ListImagesResponse{Images: pb_images, Count: int64(len(images))}, nil
}

func (s *ImageServer) PullImage(ctx context.Context, in *pb.PullImageRequest) (*pb.PullImageResponse, error) {
	ns := namespaces.WithNamespace(context.Background(), in.GetNamespace())

	image, err := s.client.Pull(ns, in.GetName(), containerd.WithPullUnpack)
	if err != nil {
		log.Printf("Failed to pull image: %v", err)
		return nil, status.Error(codes.Aborted, err.Error())
	}

	size, err := image.Size(ns)
	if err != nil {
		log.Printf("Failed to get Size: %v", err)
	}

	return &pb.PullImageResponse{Image: &pb.Image{Name: image.Name(), Size: size}}, nil
}

func (s *ImageServer) DeleteImage(ctx context.Context, in *pb.DeleteImageRequest) (*emptypb.Empty, error) {
	ns := namespaces.WithNamespace(context.Background(), in.GetNamespace())
	err := s.client.ImageService().Delete(ns, in.GetImageName())
	if err != nil {
		log.Printf("Failed to delete image: %v", err)
		return nil, status.Error(codes.Aborted, err.Error())
	}

	return &emptypb.Empty{}, nil
}

func (s *NamespaceServer) ListNamespaces(ctx context.Context, _ *emptypb.Empty) (*pb.ListNamespacesResponse, error) {

	ns := s.client.NamespaceService()
	namespaces, err := ns.List(ctx)
	if err != nil {
		log.Printf("Failed to list namespace: %v", err.Error())
	}

	return &pb.ListNamespacesResponse{Namespaces: namespaces, Count: int64(len(namespaces))}, nil
}

func (s *ContainerServer) ListContainers(ctx context.Context, in *pb.ListContainersRequest) (*pb.ListContainersResponse, error) {
	ns := namespaces.WithNamespace(context.Background(), in.GetNamespace())

	containers, err := s.client.Containers(ns)
	if err != nil {
		log.Printf("Failed to get containers: %v", err)
		return nil, status.Error(codes.Aborted, err.Error())
	}

	pb_containers := []*pb.Container{}

	for _, container := range containers {
		container_image, err := container.Image(ns)
		if err != nil {
			log.Printf("Failed to get container image: %v", err)
			return nil, status.Error(codes.Aborted, err.Error())
		}

		container_image_size, err := container_image.Size(ns)
		if err != nil {
			log.Printf("Failed to read size of image from container: %v", err)
			return nil, status.Error(codes.Aborted, err.Error())
		}

		container_labels, err := container.Labels(ns)
		if err != nil {
			log.Printf("Failed to read labels of container: %v", err)
			return nil, status.Error(codes.Aborted, err.Error())
		}

		pb_containers = append(pb_containers[:], &pb.Container{
			Id: container.ID(),
			Image: &pb.Image{
				Name:   container_image.Name(),
				Labels: container_image.Labels(),
				Size:   container_image_size,
			},
			Labels: container_labels,
		})
	}

	return &pb.ListContainersResponse{Containers: pb_containers, Count: int64(len(containers))}, nil
}

func (s *ContainerServer) CreateContainer(ctx context.Context, in *pb.CreateContainerRequest) (*pb.CreateContainerResponse, error) {
	ns := namespaces.WithNamespace(context.Background(), in.GetNamespace())
	image, err := s.client.GetImage(ns, in.GetImageName())
	if err != nil {
		log.Printf("Failed to get image: %v", err)
		return nil, status.Error(codes.Aborted, err.Error())
	}
	if image == nil {
		log.Printf("Not found image with name: %v", in.GetImageName())
		return nil, status.Error(codes.NotFound, "Not found image with this name")
	}

	image_size, err := image.Size(ns)
	if err != nil {
		log.Printf("Failed to read size of image %v", err)
		return nil, status.Error(codes.Aborted, err.Error())
	}

	container, err := s.client.NewContainer(
		ns,
		in.GetName(),
		containerd.WithImage(image),
		containerd.WithNewSnapshot(in.GetName()+"-snapshot", image),
		containerd.WithNewSpec(oci.WithImageConfig(image)),
	)
	if err != nil {
		log.Printf("Failed to create new container: %v", err)
		return nil, status.Error(codes.Aborted, err.Error())
	}

	container_labels, err := container.Labels(ns)
	if err != nil {
		log.Printf("Failed to read labels from container %v", err)
		return nil, status.Error(codes.Aborted, err.Error())
	}

	return &pb.CreateContainerResponse{
		Container: &pb.Container{
			Id: container.ID(),
			Image: &pb.Image{
				Name:   image.Name(),
				Labels: image.Labels(),
				Size:   image_size,
			},
			Labels: container_labels,
		},
	}, nil
}

func (s *ContainerServer) DeleteContainer(ctx context.Context, in *pb.DeleteContainerRequest) (*emptypb.Empty, error) {
	ns := namespaces.WithNamespace(context.Background(), in.GetNamespace())
	err := s.client.ContainerService().Delete(ns, in.GetContainerId())
	if err != nil {
		log.Printf("Failed to delete container: %v", err)
		return nil, status.Error(codes.Aborted, err.Error())
	}

	return &emptypb.Empty{}, nil
}

func (s *TaskServer) ListTasks(ctx context.Context, in *pb.ListTasksRequest) (*pb.ListTasksResponse, error) {
	ns := namespaces.WithNamespace(ctx, in.GetNamespace())
	task_list, err := s.client.TaskService().List(ns, &tasks.ListTasksRequest{})
	if err != nil {
		log.Printf("Failed to list tasks: %v", err)
		return nil, status.Error(codes.Aborted, err.Error())
	}

	return &pb.ListTasksResponse{Count: int64(len(task_list.Tasks))}, nil
}

func (s *TaskServer) TailLog(in *pb.TailLogRequest, stream pb.TaskService_TailLogServer) error {
	select {
	case <-stream.Context().Done():
		return nil

	default:
		ns := namespaces.WithNamespace(context.Background(), in.GetNamespace())
		container, err := s.client.LoadContainer(ns, in.GetContainerId())
		if err != nil {
			log.Printf("Failed to get container: %v", err)
			return status.Error(codes.Aborted, err.Error())
		}
		//todo
		pr, pw := io.Pipe()
		defer pw.Close()
		defer pr.Close()
		task, err := container.NewTask(ns, cio.NewCreator(cio.WithStreams(pr, pw, nil)))

		if err != nil {
			log.Printf("Failed to create log task: %v", err)
			return status.Error(codes.Aborted, err.Error())
		}
		defer task.Delete(ns)

		buffer := make([]byte, 4)

		for {
			n, err := pr.Read(buffer)
			if err != nil {
				if err == io.EOF {
					break
				}
				log.Printf("Failed to reading byte from read buffer: %v", err)
				return status.Error(codes.Aborted, err.Error())
			}
			log.Println(n)
			if err := stream.Send(&pb.TailLogResponse{Logs: string(buffer[:n])}); err != nil {
				log.Printf("Failed to stend stream: %v", err)
				return status.Error(codes.Aborted, err.Error())
			}
		}

		exitStatusC, err := task.Wait(ns)
		if err != nil {
			log.Printf("Failed to wait task: %v", err)
			return status.Error(codes.Aborted, err.Error())
		}

		if err := task.Start(ns); err != nil {
			log.Printf("Failed to start task: %v", err)
			return status.Error(codes.Aborted, err.Error())
		}

		time.Sleep(10 * time.Second)

		if err := task.Kill(ns, syscall.SIGTERM); err != nil {
			log.Printf("Failed to kill task: %v", err)
			return status.Error(codes.Aborted, err.Error())
		}

		exit_status := <-exitStatusC
		code, _, err := exit_status.Result()
		if err != nil {
			log.Printf("Failed to get status result: %v", err)
			return status.Error(codes.Aborted, err.Error())
		}
		log.Printf("Task exited with code: %v", code)

		return nil
	}
}

func main() {
	flag.Parse()

	client, err := containerd.New("/run/containerd/containerd.sock")
	if err != nil {
		log.Fatalf("Failed to start containerd: %v", err)
	}
	defer client.Close()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))

	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterImageServiceServer(s, &ImageServer{client: client})
	pb.RegisterNamespaceServiceServer(s, &NamespaceServer{client: client})
	pb.RegisterTaskServiceServer(s, &TaskServer{client: client})
	pb.RegisterContainerServiceServer(s, &ContainerServer{client: client})

	log.Printf("Server listening on %v", *port)

	if err := s.Serve(lis); err != nil {
		log.Fatalf("Fail to serve: %v", err)
	}
}
