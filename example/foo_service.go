package main

import "time"

type FooSvc int

type FooArgs struct {
	Num1, Num2 int
}

func (f FooSvc) Sum(args FooArgs, reply *int) error {
	*reply = args.Num1 + args.Num2
	return nil
}

func (f FooSvc) Sleep(args FooArgs, reply *int) error {
	time.Sleep(time.Second * time.Duration(args.Num1))
	*reply = args.Num1 + args.Num2
	return nil
}
